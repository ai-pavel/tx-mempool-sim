package mempool

import "container/heap"

// txHeap implements heap.Interface for transactions ordered by gas price (max-heap).
type txHeap []*Transaction

func (h txHeap) Len() int { return len(h) }

// Less returns true when i has higher priority than j.
// Higher gas price = higher priority. Ties broken by earlier timestamp.
func (h txHeap) Less(i, j int) bool {
	if h[i].GasPrice == h[j].GasPrice {
		return h[i].Timestamp < h[j].Timestamp
	}
	return h[i].GasPrice > h[j].GasPrice
}

func (h txHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *txHeap) Push(x interface{}) {
	*h = append(*h, x.(*Transaction))
}

func (h *txHeap) Pop() interface{} {
	old := *h
	n := len(old)
	tx := old[n-1]
	old[n-1] = nil // avoid memory leak
	*h = old[:n-1]
	return tx
}

// PriorityQueue wraps a heap to manage transactions by gas price.
type PriorityQueue struct {
	h txHeap
}

// NewPriorityQueue creates an empty priority queue.
func NewPriorityQueue() *PriorityQueue {
	pq := &PriorityQueue{}
	heap.Init(&pq.h)
	return pq
}

// Push adds a transaction to the queue.
func (pq *PriorityQueue) Push(tx *Transaction) {
	heap.Push(&pq.h, tx)
}

// Pop removes and returns the highest-priority transaction.
func (pq *PriorityQueue) Pop() *Transaction {
	if pq.h.Len() == 0 {
		return nil
	}
	return heap.Pop(&pq.h).(*Transaction)
}

// Peek returns the highest-priority transaction without removing it.
func (pq *PriorityQueue) Peek() *Transaction {
	if pq.h.Len() == 0 {
		return nil
	}
	return pq.h[0]
}

// Len returns the number of transactions in the queue.
func (pq *PriorityQueue) Len() int {
	return pq.h.Len()
}

// RemoveLowest removes and returns the lowest gas-price transaction.
// This is used for eviction when the pool is full.
func (pq *PriorityQueue) RemoveLowest() *Transaction {
	if pq.h.Len() == 0 {
		return nil
	}
	// Find the element with the lowest gas price.
	minIdx := 0
	for i := 1; i < pq.h.Len(); i++ {
		if pq.h[i].GasPrice < pq.h[minIdx].GasPrice {
			minIdx = i
		} else if pq.h[i].GasPrice == pq.h[minIdx].GasPrice && pq.h[i].Timestamp > pq.h[minIdx].Timestamp {
			minIdx = i
		}
	}
	return heap.Remove(&pq.h, minIdx).(*Transaction)
}

// RemoveByHash removes the transaction with the given hash from the queue,
// returning it, or nil if it was not present.
func (pq *PriorityQueue) RemoveByHash(hash string) *Transaction {
	for i := 0; i < pq.h.Len(); i++ {
		if pq.h[i].Hash == hash {
			return heap.Remove(&pq.h, i).(*Transaction)
		}
	}
	return nil
}

// All returns a copy of all transactions in the queue (unordered).
func (pq *PriorityQueue) All() []*Transaction {
	result := make([]*Transaction, len(pq.h))
	copy(result, pq.h)
	return result
}
