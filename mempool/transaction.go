// Package mempool implements a blockchain transaction mempool with
// priority-based ordering, size-limited eviction, and nonce-gap detection.
package mempool

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/crypto/sha3"
)

// Transaction represents a pending blockchain transaction.
type Transaction struct {
	Hash      string `json:"hash"`
	Sender    string `json:"sender"`
	Nonce     uint64 `json:"nonce"`
	GasPrice  uint64 `json:"gasPrice"`
	Size      uint64 `json:"size"`
	Timestamp int64  `json:"timestamp"`

	// maxIdx and minIdx track this transaction's position within the
	// PriorityQueue's max-heap (priority) and min-heap (floor) respectively.
	// They are maintained by the heaps so that O(log n) removal of an
	// arbitrary element is possible. They are not part of the wire format.
	maxIdx int `json:"-"`
	minIdx int `json:"-"`
}

// NewTransaction creates a transaction and computes its hash from the content.
func NewTransaction(sender string, nonce, gasPrice, size uint64) *Transaction {
	tx := &Transaction{
		Sender:    sender,
		Nonce:     nonce,
		GasPrice:  gasPrice,
		Size:      size,
		Timestamp: time.Now().UnixNano(),
	}
	tx.Hash = tx.computeHash()
	return tx
}

// computeHash derives a deterministic hash from the transaction fields using SHA3-256.
func (tx *Transaction) computeHash() string {
	data := fmt.Sprintf("%s:%d:%d:%d:%d", tx.Sender, tx.Nonce, tx.GasPrice, tx.Size, tx.Timestamp)
	h := sha3.New256()
	h.Write([]byte(data))
	return "0x" + hex.EncodeToString(h.Sum(nil))
}

// MarshalJSON implements custom JSON marshalling.
func (tx *Transaction) MarshalJSON() ([]byte, error) {
	type Alias Transaction
	return json.Marshal(&struct{ *Alias }{Alias: (*Alias)(tx)})
}
