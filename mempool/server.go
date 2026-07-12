package mempool

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
}

// NewServer creates a new JSON-RPC server backed by the given pool.
func NewServer(pool *Pool) *Server {
	s := &Server{pool: pool, mux: http.NewServeMux()}
	s.mux.HandleFunc("/", s.handleRPC)
	s.mux.HandleFunc("/health", s.handleHealth)
	return s
}

// handleHealth implements the fleet-wide uniform health endpoint.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, `{"status":"ok","service":"purgatory"}`)
}

// Handler returns the http.Handler for this server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// ListenAndServe starts the HTTP server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

func (s *Server) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, errorResponse(nil, -32700, "parse error"))
		return
	}
	defer r.Body.Close()

	// Detect a JSON-RPC 2.0 batch: a top-level JSON array of request objects.
	if isBatch(body) {
		var reqs []JSONRPCRequest
		if err := json.Unmarshal(body, &reqs); err != nil {
			writeJSON(w, errorResponse(nil, -32700, "parse error"))
			return
		}
		if len(reqs) == 0 {
			// An empty batch is itself an invalid request per the spec.
			writeJSON(w, errorResponse(nil, -32600, "invalid request: empty batch"))
			return
		}
		responses := make([]JSONRPCResponse, 0, len(reqs))
		for i := range reqs {
			responses = append(responses, s.dispatch(&reqs[i]))
		}
		writeJSON(w, responses)
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, errorResponse(nil, -32700, "parse error"))
		return
	}
	writeJSON(w, s.dispatch(&req))
}

// isBatch reports whether the body's first non-whitespace byte is '[',
// indicating a JSON-RPC batch (array of requests).
func isBatch(body []byte) bool {
	for _, b := range body {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '[':
			return true
		default:
			return false
		}
	}
	return false
}

// dispatch validates and routes a single JSON-RPC request, returning the
// response object rather than writing it, so it can be reused for batches.
func (s *Server) dispatch(req *JSONRPCRequest) JSONRPCResponse {
	if req.JSONRPC != "2.0" {
		return errorResponse(req.ID, -32600, "invalid request: jsonrpc must be 2.0")
	}

	switch req.Method {
	case "sendTransaction":
		return s.handleSendTransaction(req)
	case "getPoolStatus":
		return s.handleGetPoolStatus(req)
	case "getPendingByAddress":
		return s.handleGetPendingByAddress(req)
	default:
		return errorResponse(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleSendTransaction(req *JSONRPCRequest) JSONRPCResponse {
	var params SendTransactionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	if params.Sender == "" {
		return errorResponse(req.ID, -32602, "invalid params: sender is required")
	}
	if params.Size == 0 {
		params.Size = 100 // default size in bytes
	}

	tx := NewTransaction(params.Sender, params.Nonce, params.GasPrice, params.Size)
	if err := s.pool.Add(tx); err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}

	return resultResponse(req.ID, map[string]interface{}{
		"hash":    tx.Hash,
		"message": "transaction accepted",
	})
}

func (s *Server) handleGetPoolStatus(req *JSONRPCRequest) JSONRPCResponse {
	status := s.pool.Status()
	return resultResponse(req.ID, status)
}

func (s *Server) handleGetPendingByAddress(req *JSONRPCRequest) JSONRPCResponse {
	var params GetPendingByAddressParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
	}

	if params.Address == "" {
		return errorResponse(req.ID, -32602, "invalid params: address is required")
	}

	txs := s.pool.PendingByAddress(params.Address)
	if txs == nil {
		txs = []*Transaction{}
	}
	return resultResponse(req.ID, txs)
}

// resultResponse builds a successful JSON-RPC response object.
func resultResponse(id interface{}, result interface{}) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
}

// errorResponse builds a JSON-RPC error response object.
func errorResponse(id interface{}, code int, message string) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: "2.0",
		Error:   &RPCError{Code: code, Message: message},
		ID:      id,
	}
}

// writeJSON serializes v as JSON to the response writer.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
