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
