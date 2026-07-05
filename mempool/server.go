package mempool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Default HTTP server timeouts. These bound how long a single connection may
// tie up server resources, mitigating slow-client (Slowloris) DoS.
const (
	DefaultReadTimeout       = 15 * time.Second
	DefaultReadHeaderTimeout = 5 * time.Second
	DefaultWriteTimeout      = 15 * time.Second
	DefaultIdleTimeout       = 60 * time.Second
)

// JSONRPCRequest represents an incoming JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      interface{}     `json:"id"`
}

// JSONRPCResponse represents an outgoing JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// RPCError represents a JSON-RPC error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// SendTransactionParams holds the parameters for sendTransaction.
type SendTransactionParams struct {
	Sender   string `json:"sender"`
	Nonce    uint64 `json:"nonce"`
	GasPrice uint64 `json:"gasPrice"`
	Size     uint64 `json:"size"`
}

// GetPendingByAddressParams holds the parameters for getPendingByAddress.
type GetPendingByAddressParams struct {
	Address string `json:"address"`
}

// Server is the JSON-RPC HTTP server for the mempool.
type Server struct {
	pool *Pool
	mux  *http.ServeMux
	http *http.Server
}

// NewServer creates a new JSON-RPC server backed by the given pool.
func NewServer(pool *Pool) *Server {
	s := &Server{pool: pool, mux: http.NewServeMux()}
	s.mux.HandleFunc("/", s.handleRPC)
	return s
}

// Handler returns the http.Handler for this server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// ListenAndServe starts the HTTP server on the given address using an
// explicit http.Server configured with read/write/idle timeouts, rather than
// the default (timeout-less) http.ListenAndServe.
func (s *Server) ListenAndServe(addr string) error {
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadTimeout:       DefaultReadTimeout,
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
		WriteTimeout:      DefaultWriteTimeout,
		IdleTimeout:       DefaultIdleTimeout,
	}
	return s.http.ListenAndServe()
}

// Shutdown gracefully shuts down the underlying HTTP server, letting
// in-flight requests finish before the deadline in ctx elapses. It is safe to
// call before ListenAndServe (in which case it is a no-op).
func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, nil, -32700, "parse error")
		return
	}
	defer r.Body.Close()

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, nil, -32700, "parse error")
		return
	}

	if req.JSONRPC != "2.0" {
		writeError(w, req.ID, -32600, "invalid request: jsonrpc must be 2.0")
		return
	}

	switch req.Method {
	case "sendTransaction":
		s.handleSendTransaction(w, &req)
	case "getPoolStatus":
		s.handleGetPoolStatus(w, &req)
	case "getPendingByAddress":
		s.handleGetPendingByAddress(w, &req)
	default:
		writeError(w, req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleSendTransaction(w http.ResponseWriter, req *JSONRPCRequest) {
	var params SendTransactionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -32602, "invalid params: "+err.Error())
		return
	}

	if params.Sender == "" {
		writeError(w, req.ID, -32602, "invalid params: sender is required")
		return
	}
	if params.Size == 0 {
		params.Size = 100 // default size in bytes
	}

	tx := NewTransaction(params.Sender, params.Nonce, params.GasPrice, params.Size)
	if err := s.pool.Add(tx); err != nil {
		writeError(w, req.ID, -32000, err.Error())
		return
	}

	writeResult(w, req.ID, map[string]interface{}{
		"hash":    tx.Hash,
		"message": "transaction accepted",
	})
}

func (s *Server) handleGetPoolStatus(w http.ResponseWriter, req *JSONRPCRequest) {
	status := s.pool.Status()
	writeResult(w, req.ID, status)
}

func (s *Server) handleGetPendingByAddress(w http.ResponseWriter, req *JSONRPCRequest) {
	var params GetPendingByAddressParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -32602, "invalid params: "+err.Error())
		return
	}

	if params.Address == "" {
		writeError(w, req.ID, -32602, "invalid params: address is required")
		return
	}

	txs := s.pool.PendingByAddress(params.Address)
	if txs == nil {
		txs = []*Transaction{}
	}
	writeResult(w, req.ID, txs)
}

func writeResult(w http.ResponseWriter, id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		Error:   &RPCError{Code: code, Message: message},
		ID:      id,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
