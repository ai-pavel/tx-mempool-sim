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

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxSize != 5000 {
		t.Errorf("expected default MaxSize 5000, got %d", cfg.MaxSize)
	}
}

func TestNewPoolDefaultsOnInvalidMaxSize(t *testing.T) {
	// MaxSize of 0 should be replaced with the default.
	pool := NewPool(Config{MaxSize: 0})
	status := pool.Status()
	if status.MaxSize != 5000 {
		t.Errorf("expected MaxSize 5000 for zero config, got %d", status.MaxSize)
	}

	// Negative MaxSize should also be replaced.
	pool2 := NewPool(Config{MaxSize: -1})
	status2 := pool2.Status()
	if status2.MaxSize != 5000 {
		t.Errorf("expected MaxSize 5000 for negative config, got %d", status2.MaxSize)
	}
}

func TestEvictionRejectsEqualGasPrice(t *testing.T) {
	pool := NewPool(Config{MaxSize: 1})

	tx1 := NewTransaction("0xAlice", 0, 50, 100)
	if err := pool.Add(tx1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Adding a tx with the same gas price should be rejected (not strictly greater).
	tx2 := NewTransaction("0xBob", 0, 50, 100)
	if err := pool.Add(tx2); err == nil {
		t.Error("expected error when gas price equals floor in full pool")
	}

	// Original should still be there.
	status := pool.Status()
	if status.Size != 1 {
		t.Errorf("expected size 1, got %d", status.Size)
	}
}

func TestEvictionMultipleRounds(t *testing.T) {
	pool := NewPool(Config{MaxSize: 2})

	tx1 := NewTransaction("0xA", 0, 10, 100)
	tx2 := NewTransaction("0xB", 0, 20, 100)
	pool.Add(tx1)
	pool.Add(tx2)

	// Evict gas price 10 with gas price 15.
	tx3 := NewTransaction("0xC", 0, 15, 100)
	if err := pool.Add(tx3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Evict gas price 15 with gas price 25.
	tx4 := NewTransaction("0xD", 0, 25, 100)
	if err := pool.Add(tx4); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := pool.Status()
	if status.Size != 2 {
		t.Errorf("expected size 2, got %d", status.Size)
	}
	if status.FloorGasPrice != 20 {
		t.Errorf("expected floor 20, got %d", status.FloorGasPrice)
	}
	if status.TopGasPrice != 25 {
		t.Errorf("expected top 25, got %d", status.TopGasPrice)
	}
}

func TestEvictionRemovesSenderIndex(t *testing.T) {
	pool := NewPool(Config{MaxSize: 1})

	tx1 := NewTransaction("0xAlice", 0, 10, 100)
	pool.Add(tx1)

	// Evict Alice's tx.
	tx2 := NewTransaction("0xBob", 0, 20, 100)
	pool.Add(tx2)

	// Alice should have no pending transactions.
	pending := pool.PendingByAddress("0xAlice")
	if pending != nil {
		t.Errorf("expected nil for evicted sender, got %d txs", len(pending))
	}
}

func TestPendingByAddressReturnsCopy(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	tx := NewTransaction("0xAlice", 0, 50, 100)
	pool.Add(tx)

	// Modifying the returned slice should not affect the pool.
	pending := pool.PendingByAddress("0xAlice")
	pending[0] = nil

	pending2 := pool.PendingByAddress("0xAlice")
	if pending2[0] == nil {
		t.Error("PendingByAddress should return a copy, not the internal slice")
	}
}

func TestNonceGapDetectionNoGaps(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	// Add sequential nonces.
	for n := uint64(0); n < 5; n++ {
		tx := NewTransaction("0xAlice", n, 50, 100)
		pool.Add(tx)
	}

	gaps := pool.DetectNonceGaps()
	if len(gaps) != 0 {
		t.Errorf("expected no gaps, got %d", len(gaps))
	}
}

func TestNonceGapDetectionSingleTx(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	tx := NewTransaction("0xAlice", 5, 50, 100)
	pool.Add(tx)

	// Single tx per sender should never report gaps.
	gaps := pool.DetectNonceGaps()
	if len(gaps) != 0 {
		t.Errorf("expected no gaps for single tx, got %d", len(gaps))
	}
}

func TestNonceGapDetectionMultipleGaps(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	// Nonces 0, 3, 7 => gaps at 1 and 4.
	for _, n := range []uint64{0, 3, 7} {
		tx := NewTransaction("0xAlice", n, 50, 100)
		pool.Add(tx)
	}

	gaps := pool.DetectNonceGaps()
	if len(gaps) != 2 {
		t.Fatalf("expected 2 gaps, got %d", len(gaps))
	}
	if gaps[0].Expected != 1 || gaps[0].Found != 3 {
		t.Errorf("gap 0: expected (1,3), got (%d,%d)", gaps[0].Expected, gaps[0].Found)
	}
	if gaps[1].Expected != 4 || gaps[1].Found != 7 {
		t.Errorf("gap 1: expected (4,7), got (%d,%d)", gaps[1].Expected, gaps[1].Found)
	}
}

func TestNonceGapDetectionMultipleSenders(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	// Alice: nonces 0, 2 (gap at 1)
	pool.Add(NewTransaction("0xAlice", 0, 50, 100))
	pool.Add(NewTransaction("0xAlice", 2, 50, 100))

	// Bob: nonces 0, 1 (no gap)
	pool.Add(NewTransaction("0xBob", 0, 50, 100))
	pool.Add(NewTransaction("0xBob", 1, 50, 100))

	gaps := pool.DetectNonceGaps()
	if len(gaps) != 1 {
		t.Fatalf("expected 1 gap, got %d", len(gaps))
	}
	if gaps[0].Sender != "0xAlice" {
		t.Errorf("expected gap for Alice, got %s", gaps[0].Sender)
	}
}

func TestStatusEmptyPool(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})
	status := pool.Status()

	if status.Size != 0 {
		t.Errorf("expected size 0, got %d", status.Size)
	}
	if status.SenderCount != 0 {
		t.Errorf("expected 0 senders, got %d", status.SenderCount)
	}
	if status.TopGasPrice != 0 {
		t.Errorf("expected top gas price 0, got %d", status.TopGasPrice)
	}
	if status.FloorGasPrice != 0 {
		t.Errorf("expected floor gas price 0, got %d", status.FloorGasPrice)
	}
	if status.MaxSize != 100 {
		t.Errorf("expected maxSize 100, got %d", status.MaxSize)
	}
}

func TestStatusIncludesNonceGaps(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	pool.Add(NewTransaction("0xAlice", 0, 50, 100))
	pool.Add(NewTransaction("0xAlice", 3, 50, 100))

	status := pool.Status()
	if len(status.NonceGaps) != 1 {
		t.Fatalf("expected 1 nonce gap in status, got %d", len(status.NonceGaps))
	}
	if status.NonceGaps[0].Expected != 1 {
		t.Errorf("expected gap at nonce 1, got %d", status.NonceGaps[0].Expected)
	}
}

func TestPoolConcurrentAccess(t *testing.T) {
	pool := NewPool(Config{MaxSize: 1000})
	done := make(chan struct{})

	// Spawn multiple goroutines adding transactions concurrently.
	for g := 0; g < 10; g++ {
		go func(gid int) {
			defer func() { done <- struct{}{} }()
			sender := fmt.Sprintf("0xSender%d", gid)
			for i := 0; i < 50; i++ {
				tx := NewTransaction(sender, uint64(i), uint64(10+i), 100)
				pool.Add(tx)
			}
		}(g)
	}

	// Spawn readers concurrently.
	for g := 0; g < 5; g++ {
		go func(gid int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 50; i++ {
				pool.Status()
				pool.PendingByAddress(fmt.Sprintf("0xSender%d", gid))
				pool.DetectNonceGaps()
			}
		}(g)
	}

	// Wait for all goroutines.
	for i := 0; i < 15; i++ {
		<-done
	}

	status := pool.Status()
	if status.Size == 0 {
		t.Error("expected some transactions to be added")
	}
	if status.SenderCount == 0 {
		t.Error("expected some senders")
	}
}

func TestPoolMaxSizeOne(t *testing.T) {
	pool := NewPool(Config{MaxSize: 1})

	tx1 := NewTransaction("0xAlice", 0, 10, 100)
	if err := pool.Add(tx1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tx2 := NewTransaction("0xBob", 0, 20, 100)
	if err := pool.Add(tx2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status := pool.Status()
	if status.Size != 1 {
		t.Errorf("expected size 1, got %d", status.Size)
	}

	// Only Bob should remain.
	if pool.PendingByAddress("0xAlice") != nil {
		t.Error("Alice should have been evicted")
	}
	pending := pool.PendingByAddress("0xBob")
	if len(pending) != 1 {
		t.Errorf("expected 1 tx for Bob, got %d", len(pending))
	}
}

func TestAddMultipleSenders(t *testing.T) {
	pool := NewPool(Config{MaxSize: 100})

	senders := []string{"0xAlice", "0xBob", "0xCharlie"}
	for _, sender := range senders {
		for n := uint64(0); n < 3; n++ {
			tx := NewTransaction(sender, n, 50, 100)
			if err := pool.Add(tx); err != nil {
				t.Fatalf("unexpected error for %s nonce %d: %v", sender, n, err)
			}
		}
	}

	status := pool.Status()
	if status.Size != 9 {
		t.Errorf("expected 9 txs, got %d", status.Size)
	}
	if status.SenderCount != 3 {
		t.Errorf("expected 3 senders, got %d", status.SenderCount)
	}
}

func TestEvictionDoesNotCreateNonceGap(t *testing.T) {
	// Capacity 2. Alice's nonce-0 (cheap) and nonce-1 (also cheap) fill the pool.
	pool := NewPool(Config{MaxSize: 2})
	tx0 := NewTransaction("0xAlice", 0, 10, 100)
	tx1 := NewTransaction("0xAlice", 1, 10, 100)
	if err := pool.Add(tx0); err != nil {
		t.Fatalf("add tx0: %v", err)
	}
	if err := pool.Add(tx1); err != nil {
		t.Fatalf("add tx1: %v", err)
	}

	// A higher-priced tx from Bob forces an eviction. The cheapest candidate
	// could be Alice's nonce-0, but evicting it would strand nonce-1. The
	// pool should instead drop Alice's tail (nonce-1), leaving no gap.
	bob := NewTransaction("0xBob", 0, 100, 100)
	if err := pool.Add(bob); err != nil {
		t.Fatalf("add bob: %v", err)
	}

	alice := pool.PendingByAddress("0xAlice")
	found0 := false
	for _, tx := range alice {
		if tx.Nonce == 0 {
			found0 = true
		}
	}
	if !found0 {
		t.Error("nonce-0 should have been retained to avoid a gap")
	}
	if len(pool.DetectNonceGaps()) != 0 {
		t.Errorf("eviction created a nonce gap: %v", pool.DetectNonceGaps())
	}
}
