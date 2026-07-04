package mempool

import (
	"fmt"
	"testing"
)

func TestAddAndRetrieve(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	tx := NewTransaction("0xAlice", 0, 50, 200)
	if err := pool.Add(tx); err != nil {
		t.Fatalf("unexpected error adding tx: %v", err)
	}

	pending := pool.PendingByAddress("0xAlice")
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending tx, got %d", len(pending))
	}
	if pending[0].Hash != tx.Hash {
		t.Errorf("hash mismatch: got %s, want %s", pending[0].Hash, tx.Hash)
	}
}

func TestDuplicateRejection(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	tx := NewTransaction("0xAlice", 0, 50, 200)
	if err := pool.Add(tx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := pool.Add(tx); err == nil {
		t.Fatal("expected error for duplicate transaction")
	}
}

func TestDuplicateNonceRejection(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	tx1 := NewTransaction("0xAlice", 0, 50, 200)
	tx2 := NewTransaction("0xAlice", 0, 100, 200) // same nonce, different gas price
	if err := pool.Add(tx1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := pool.Add(tx2); err == nil {
		t.Fatal("expected error for duplicate nonce")
	}
}

func TestEviction(t *testing.T) {
	pool := NewPool(Config{MaxSize: 3})

	// Fill the pool with gas prices 10, 20, 30.
	for i := 0; i < 3; i++ {
		tx := NewTransaction("0xBob", uint64(i), uint64((i+1)*10), 100)
		if err := pool.Add(tx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Adding a tx with gas price 5 should fail (below floor of 10).
	lowTx := NewTransaction("0xCharlie", 0, 5, 100)
	if err := pool.Add(lowTx); err == nil {
		t.Fatal("expected error for low gas price when pool is full")
	}

	// Adding a tx with gas price 15 should succeed and evict gas price 10.
	highTx := NewTransaction("0xCharlie", 0, 15, 100)
	if err := pool.Add(highTx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := pool.Status()
	if status.Size != 3 {
		t.Errorf("expected pool size 3, got %d", status.Size)
	}
	if status.FloorGasPrice != 15 {
		t.Errorf("expected floor gas price 15, got %d", status.FloorGasPrice)
	}
}

func TestNonceGapDetection(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	// Add nonces 0, 1, 3 (skipping 2).
	for _, n := range []uint64{0, 1, 3} {
		tx := NewTransaction("0xDave", n, 50, 100)
		if err := pool.Add(tx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	gaps := pool.DetectNonceGaps()
	if len(gaps) != 1 {
		t.Fatalf("expected 1 gap, got %d", len(gaps))
	}
	if gaps[0].Expected != 2 || gaps[0].Found != 3 {
		t.Errorf("unexpected gap: expected nonce 2, found %d (gap says expected=%d, found=%d)",
			2, gaps[0].Expected, gaps[0].Found)
	}
}

func TestStatus(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	for i := 0; i < 5; i++ {
		tx := NewTransaction(fmt.Sprintf("0xUser%d", i), 0, uint64(10+i*5), 100)
		if err := pool.Add(tx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	status := pool.Status()
	if status.Size != 5 {
		t.Errorf("expected size 5, got %d", status.Size)
	}
	if status.SenderCount != 5 {
		t.Errorf("expected 5 senders, got %d", status.SenderCount)
	}
	if status.TopGasPrice != 30 {
		t.Errorf("expected top gas price 30, got %d", status.TopGasPrice)
	}
	if status.FloorGasPrice != 10 {
		t.Errorf("expected floor gas price 10, got %d", status.FloorGasPrice)
	}
}

func TestPendingByAddressEmpty(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	txs := pool.PendingByAddress("0xNobody")
	if txs != nil {
		t.Errorf("expected nil for unknown address, got %v", txs)
	}
}

func TestPendingByAddressSorted(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	// Add nonces out of order.
	for _, n := range []uint64{3, 1, 2, 0} {
		tx := NewTransaction("0xEve", n, 50, 100)
		if err := pool.Add(tx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	txs := pool.PendingByAddress("0xEve")
	if len(txs) != 4 {
		t.Fatalf("expected 4 txs, got %d", len(txs))
	}
	for i, tx := range txs {
		if tx.Nonce != uint64(i) {
			t.Errorf("expected nonce %d at index %d, got %d", i, i, tx.Nonce)
		}
	}
}
