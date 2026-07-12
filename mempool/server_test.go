package mempool

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func rpcCall(t *testing.T, handler http.Handler, method string, params interface{}) JSONRPCResponse {
	t.Helper()
	paramsJSON, _ := json.Marshal(params)
	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsJSON,
		ID:      1,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return resp
}

func TestHealthEndpoint(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	want := `{"status":"ok","service":"purgatory"}`
	if got := w.Body.String(); got != want {
		t.Errorf("expected body %s, got %s", want, got)
	}
}

func TestHealthEndpointMethodNotAllowed(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestSendTransactionRPC(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	resp := rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender:   "0xAlice",
		Nonce:    0,
		GasPrice: 50,
		Size:     200,
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}
	if result["hash"] == nil || result["hash"] == "" {
		t.Error("expected hash in result")
	}
}

func TestGetPoolStatusRPC(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	// Add a transaction first.
	rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender: "0xAlice", Nonce: 0, GasPrice: 50, Size: 200,
	})

	resp := rpcCall(t, srv.Handler(), "getPoolStatus", struct{}{})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}
	if result["size"].(float64) != 1 {
		t.Errorf("expected size 1, got %v", result["size"])
	}
}

func TestGetPendingByAddressRPC(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	// Add two transactions for Alice.
	rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender: "0xAlice", Nonce: 0, GasPrice: 50, Size: 200,
	})
	rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender: "0xAlice", Nonce: 1, GasPrice: 60, Size: 150,
	})

	resp := rpcCall(t, srv.Handler(), "getPendingByAddress", GetPendingByAddressParams{
		Address: "0xAlice",
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	txs, ok := resp.Result.([]interface{})
	if !ok {
		t.Fatal("result is not an array")
	}
	if len(txs) != 2 {
		t.Errorf("expected 2 txs, got %d", len(txs))
	}
}

func TestGetPendingByAddressEmptyRPC(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	resp := rpcCall(t, srv.Handler(), "getPendingByAddress", GetPendingByAddressParams{
		Address: "0xNobody",
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	txs, ok := resp.Result.([]interface{})
	if !ok {
		t.Fatal("result is not an array")
	}
	if len(txs) != 0 {
		t.Errorf("expected 0 txs, got %d", len(txs))
	}
}

func TestMethodNotAllowed(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestUnknownMethod(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	resp := rpcCall(t, srv.Handler(), "unknownMethod", struct{}{})
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected code -32601, got %d", resp.Error.Code)
	}
}

func TestIntegrationNonceGapViaRPC(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	// Add nonces 0 and 2, creating a gap.
	rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender: "0xAlice", Nonce: 0, GasPrice: 50, Size: 100,
	})
	rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender: "0xAlice", Nonce: 2, GasPrice: 50, Size: 100,
	})

	resp := rpcCall(t, srv.Handler(), "getPoolStatus", struct{}{})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}

	nonceGaps, ok := result["nonceGaps"].([]interface{})
	if !ok || len(nonceGaps) == 0 {
		t.Fatal("expected nonce gaps in status")
	}
	gap := nonceGaps[0].(map[string]interface{})
	if gap["expected"].(float64) != 1 {
		t.Errorf("expected gap at nonce 1, got %v", gap["expected"])
	}
}

func TestIntegrationEvictionViaRPC(t *testing.T) {
	pool := NewPool(Config{MaxSize: 2})
	srv := NewServer(pool)

	// Fill pool.
	rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender: "0xA", Nonce: 0, GasPrice: 10, Size: 100,
	})
	rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender: "0xB", Nonce: 0, GasPrice: 20, Size: 100,
	})

	// This should evict gas price 10 and succeed.
	resp := rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender: "0xC", Nonce: 0, GasPrice: 15, Size: 100,
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	// Verify 0xA was evicted.
	pendingA := rpcCall(t, srv.Handler(), "getPendingByAddress", GetPendingByAddressParams{Address: "0xA"})
	txs, ok := pendingA.Result.([]interface{})
	if !ok {
		t.Fatal("result is not an array")
	}
	if len(txs) != 0 {
		t.Errorf("expected 0xA to be evicted, but found %d txs", len(txs))
	}
}

func TestSendTransactionMissingSender(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	resp := rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender:   "",
		Nonce:    0,
		GasPrice: 50,
		Size:     200,
	})

	if resp.Error == nil {
		t.Fatal("expected error for missing sender")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", resp.Error.Code)
	}
}

func TestSendTransactionDefaultSize(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	// Size 0 should default to 100.
	resp := rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender:   "0xAlice",
		Nonce:    0,
		GasPrice: 50,
		Size:     0,
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
}

func TestSendTransactionDuplicateViaRPC(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	params := SendTransactionParams{
		Sender: "0xAlice", Nonce: 0, GasPrice: 50, Size: 200,
	}
	resp := rpcCall(t, srv.Handler(), "sendTransaction", params)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	// Same nonce from same sender should fail.
	resp2 := rpcCall(t, srv.Handler(), "sendTransaction", params)
	if resp2.Error == nil {
		t.Fatal("expected error for duplicate nonce via RPC")
	}
	if resp2.Error.Code != -32000 {
		t.Errorf("expected error code -32000, got %d", resp2.Error.Code)
	}
}

func TestInvalidJSONRPC(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	// Send with wrong jsonrpc version.
	reqBody := JSONRPCRequest{
		JSONRPC: "1.0",
		Method:  "getPoolStatus",
		ID:      1,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for invalid jsonrpc version")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("expected code -32600, got %d", resp.Error.Code)
	}
}

func TestInvalidJSON(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("expected code -32700 (parse error), got %d", resp.Error.Code)
	}
}

func TestSendTransactionInvalidParams(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	// Send params that cannot be unmarshalled into SendTransactionParams.
	resp := rpcCall(t, srv.Handler(), "sendTransaction", "not an object")

	if resp.Error == nil {
		t.Fatal("expected error for invalid params")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected code -32602, got %d", resp.Error.Code)
	}
}

func TestGetPendingByAddressMissingAddress(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	resp := rpcCall(t, srv.Handler(), "getPendingByAddress", GetPendingByAddressParams{
		Address: "",
	})

	if resp.Error == nil {
		t.Fatal("expected error for missing address")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected code -32602, got %d", resp.Error.Code)
	}
}

func TestGetPendingByAddressInvalidParams(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	resp := rpcCall(t, srv.Handler(), "getPendingByAddress", "not an object")

	if resp.Error == nil {
		t.Fatal("expected error for invalid params")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("expected code -32602, got %d", resp.Error.Code)
	}
}

func TestResponseFormat(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	resp := rpcCall(t, srv.Handler(), "getPoolStatus", struct{}{})

	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %q", resp.JSONRPC)
	}
	if resp.ID != float64(1) {
		t.Errorf("expected id 1, got %v", resp.ID)
	}
}

func TestPoolStatusViaRPCFields(t *testing.T) {
	pool := NewPool(Config{MaxSize: 50})
	srv := NewServer(pool)

	// Add transactions from multiple senders with different gas prices.
	rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender: "0xA", Nonce: 0, GasPrice: 100, Size: 200,
	})
	rpcCall(t, srv.Handler(), "sendTransaction", SendTransactionParams{
		Sender: "0xB", Nonce: 0, GasPrice: 50, Size: 200,
	})

	resp := rpcCall(t, srv.Handler(), "getPoolStatus", struct{}{})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}

	result := resp.Result.(map[string]interface{})
	if result["size"].(float64) != 2 {
		t.Errorf("expected size 2, got %v", result["size"])
	}
	if result["maxSize"].(float64) != 50 {
		t.Errorf("expected maxSize 50, got %v", result["maxSize"])
	}
	if result["senderCount"].(float64) != 2 {
		t.Errorf("expected senderCount 2, got %v", result["senderCount"])
	}
	if result["topGasPrice"].(float64) != 100 {
		t.Errorf("expected topGasPrice 100, got %v", result["topGasPrice"])
	}
	if result["floorGasPrice"].(float64) != 50 {
		t.Errorf("expected floorGasPrice 50, got %v", result["floorGasPrice"])
	}
}
