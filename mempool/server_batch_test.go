package mempool

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// rawRPC posts a raw request body and decodes the response into dst.
// dst should be a pointer to either JSONRPCResponse (single) or
// []JSONRPCResponse (batch), depending on what the caller expects.
func rawRPC(t *testing.T, handler http.Handler, body string, dst interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if err := json.NewDecoder(w.Body).Decode(dst); err != nil {
		t.Fatalf("failed to decode response: %v (body: %s)", err, w.Body.String())
	}
}

func TestBatchAllSucceed(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	body := `[
		{"jsonrpc":"2.0","method":"sendTransaction","params":{"sender":"0xAlice","nonce":0,"gasPrice":50,"size":100},"id":1},
		{"jsonrpc":"2.0","method":"sendTransaction","params":{"sender":"0xAlice","nonce":1,"gasPrice":60,"size":100},"id":2},
		{"jsonrpc":"2.0","method":"getPoolStatus","params":{},"id":3}
	]`

	var resps []JSONRPCResponse
	rawRPC(t, srv.Handler(), body, &resps)

	if len(resps) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(resps))
	}
	for i, r := range resps {
		if r.Error != nil {
			t.Errorf("response %d: unexpected error: %s", i, r.Error.Message)
		}
		if r.JSONRPC != "2.0" {
			t.Errorf("response %d: expected jsonrpc 2.0, got %q", i, r.JSONRPC)
		}
	}
	// IDs should be preserved and in order.
	for i, want := range []float64{1, 2, 3} {
		if resps[i].ID != want {
			t.Errorf("response %d: expected id %v, got %v", i, want, resps[i].ID)
		}
	}

	// The pool should reflect both transactions.
	if pool.Status().Size != 2 {
		t.Errorf("expected pool size 2 after batch, got %d", pool.Status().Size)
	}
}

func TestBatchMixedSuccessAndError(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	// Second request is missing sender; third calls an unknown method.
	body := `[
		{"jsonrpc":"2.0","method":"sendTransaction","params":{"sender":"0xAlice","nonce":0,"gasPrice":50,"size":100},"id":1},
		{"jsonrpc":"2.0","method":"sendTransaction","params":{"sender":"","nonce":1,"gasPrice":60,"size":100},"id":2},
		{"jsonrpc":"2.0","method":"noSuchMethod","params":{},"id":3}
	]`

	var resps []JSONRPCResponse
	rawRPC(t, srv.Handler(), body, &resps)

	if len(resps) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Errorf("response 0: unexpected error: %s", resps[0].Error.Message)
	}
	if resps[1].Error == nil || resps[1].Error.Code != -32602 {
		t.Errorf("response 1: expected -32602, got %+v", resps[1].Error)
	}
	if resps[2].Error == nil || resps[2].Error.Code != -32601 {
		t.Errorf("response 2: expected -32601, got %+v", resps[2].Error)
	}
}

func TestBatchEmpty(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	// An empty batch is a single invalid-request error object per the spec.
	var resp JSONRPCResponse
	rawRPC(t, srv.Handler(), `[]`, &resp)

	if resp.Error == nil {
		t.Fatal("expected error for empty batch")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("expected code -32600, got %d", resp.Error.Code)
	}
}

func TestBatchMalformed(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	// Looks like a batch (leading '[') but is not valid JSON.
	var resp JSONRPCResponse
	rawRPC(t, srv.Handler(), `[ {"jsonrpc": }`, &resp)

	if resp.Error == nil {
		t.Fatal("expected parse error for malformed batch")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("expected code -32700, got %d", resp.Error.Code)
	}
}

func TestBatchLeadingWhitespace(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	// Leading whitespace before '[' must still be detected as a batch.
	body := "  \n\t [{\"jsonrpc\":\"2.0\",\"method\":\"getPoolStatus\",\"params\":{},\"id\":1}]"

	var resps []JSONRPCResponse
	rawRPC(t, srv.Handler(), body, &resps)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Errorf("unexpected error: %s", resps[0].Error.Message)
	}
}

func TestBatchSingleElement(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	srv := NewServer(pool)

	// A batch with one element must still return an array, not a bare object.
	body := `[{"jsonrpc":"2.0","method":"getPoolStatus","params":{},"id":7}]`

	var resps []JSONRPCResponse
	rawRPC(t, srv.Handler(), body, &resps)

	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0].ID != float64(7) {
		t.Errorf("expected id 7, got %v", resps[0].ID)
	}
}

func TestIsBatch(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"plain array", `[1,2]`, true},
		{"leading whitespace", "  \n\t[]", true},
		{"object", `{"a":1}`, false},
		{"leading whitespace object", "  {}", false},
		{"empty", ``, false},
		{"whitespace only", "   \n\t", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBatch([]byte(tc.body)); got != tc.want {
				t.Errorf("isBatch(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
