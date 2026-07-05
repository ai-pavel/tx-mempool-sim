package mempool

import "container/heap"

// maxHeap implements heap.Interface ordering transactions by gas price
// (highest first). Ties are broken by earlier timestamp. It maintains each
// transaction's maxIdx so arbitrary elements can be removed in O(log n).
type maxHeap []*Transaction

func (h maxHeap) Len() int { return len(h) }

// Less returns true when i has higher priority than j.
// Higher gas price = higher priority. Ties broken by earlier timestamp.
func (h maxHeap) Less(i, j int) bool {
	if h[i].GasPrice == h[j].GasPrice {
		return h[i].Timestamp < h[j].Timestamp
	}
	return h[i].GasPrice > h[j].GasPrice
}

func (h maxHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].maxIdx = i
	h[j].maxIdx = j
}

func (h *maxHeap) Push(x interface{}) {
	tx := x.(*Transaction)
	tx.maxIdx = len(*h)
	*h = append(*h, tx)
}

func (h *maxHeap) Pop() interface{} {
	old := *h
	n := len(old)
	tx := old[n-1]
	old[n-1] = nil // avoid memory leak
	tx.maxIdx = -1
	*h = old[:n-1]
	return tx
}

// minHeap implements heap.Interface ordering transactions so that the lowest
// gas-price transaction is at the root. It mirrors the eviction tie-break of
// RemoveLowest: among equally cheap transactions, the one with the later
// timestamp sorts first (is evicted first). It maintains each transaction's
// minIdx for O(log n) arbitrary removal.
type minHeap []*Transaction

func (h minHeap) Len() int { return len(h) }

func (h minHeap) Less(i, j int) bool {
	if h[i].GasPrice == h[j].GasPrice {
		return h[i].Timestamp > h[j].Timestamp
	}
	return h[i].GasPrice < h[j].GasPrice
}

func (h minHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].minIdx = i
	h[j].minIdx = j
}

func (h *minHeap) Push(x interface{}) {
	tx := x.(*Transaction)
	tx.minIdx = len(*h)
	*h = append(*h, tx)
}

func (h *minHeap) Pop() interface{} {
	old := *h
	n := len(old)
	tx := old[n-1]
	old[n-1] = nil
	tx.minIdx = -1
	*h = old[:n-1]
	return tx
}

// PriorityQueue maintains transactions in two synchronized heaps: a max-heap
// keyed by priority (gas price / timestamp) and a min-heap keyed by the
// eviction floor. This keeps Peek (top), floor lookups, and RemoveLowest all
// at O(1)/O(log n) without scanning the whole set.
type PriorityQueue struct {
	max maxHeap
	min minHeap
}

// NewPriorityQueue creates an empty priority queue.
func NewPriorityQueue() *PriorityQueue {
	pq := &PriorityQueue{}
	heap.Init(&pq.max)
	heap.Init(&pq.min)
	return pq
}

// Push adds a transaction to the queue.
func (pq *PriorityQueue) Push(tx *Transaction) {
	heap.Push(&pq.max, tx)
	heap.Push(&pq.min, tx)
}

// Pop removes and returns the highest-priority transaction.
func (pq *PriorityQueue) Pop() *Transaction {
	if pq.max.Len() == 0 {
		return nil
	}
	tx := heap.Pop(&pq.max).(*Transaction)
	heap.Remove(&pq.min, tx.minIdx)
	return tx
}

// Peek returns the highest-priority transaction without removing it.
func (pq *PriorityQueue) Peek() *Transaction {
	if pq.max.Len() == 0 {
		return nil
	}
	return pq.max[0]
}

// Floor returns the lowest gas-price transaction without removing it, or nil
// if the queue is empty. It is O(1).
func (pq *PriorityQueue) Floor() *Transaction {
	if pq.min.Len() == 0 {
		return nil
	}
	return pq.min[0]
}

// Len returns the number of transactions in the queue.
func (pq *PriorityQueue) Len() int {
	return pq.max.Len()
}

// RemoveLowest removes and returns the lowest gas-price transaction.
// This is used for eviction when the pool is full. It is O(log n).
func (pq *PriorityQueue) RemoveLowest() *Transaction {
	if pq.min.Len() == 0 {
		return nil
	}
	tx := heap.Pop(&pq.min).(*Transaction)
	heap.Remove(&pq.max, tx.maxIdx)
	return tx
}

// All returns a copy of all transactions in the queue (unordered).
func (pq *PriorityQueue) All() []*Transaction {
	result := make([]*Transaction, len(pq.max))
	copy(result, pq.max)
	return result
}
