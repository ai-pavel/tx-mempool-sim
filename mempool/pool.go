package mempool

import (
	"fmt"
	"sort"
	"sync"
)

// Config holds mempool configuration.
type Config struct {
	// MaxSize is the maximum number of transactions the pool can hold.
	MaxSize int
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxSize: 5000,
	}
}

// NonceGap describes a gap in the nonce sequence for a sender.
type NonceGap struct {
	Sender   string `json:"sender"`
	Expected uint64 `json:"expected"`
	Found    uint64 `json:"found"`
}

// PoolStatus contains summary information about the current mempool state.
type PoolStatus struct {
	Size          int        `json:"size"`
	MaxSize       int        `json:"maxSize"`
	SenderCount   int        `json:"senderCount"`
	NonceGaps     []NonceGap `json:"nonceGaps,omitempty"`
	TopGasPrice   uint64     `json:"topGasPrice"`
	FloorGasPrice uint64     `json:"floorGasPrice"`
}

// Pool is a thread-safe transaction mempool.
type Pool struct {
	mu     sync.RWMutex
	config Config
	pq     *PriorityQueue
	byHash map[string]*Transaction
	// bySender maps sender address to a sorted list of transactions by nonce.
	bySender map[string][]*Transaction
}

// NewPool creates a mempool with the given configuration.
func NewPool(cfg Config) *Pool {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = DefaultConfig().MaxSize
	}
	return &Pool{
		config:   cfg,
		pq:       NewPriorityQueue(),
		byHash:   make(map[string]*Transaction),
		bySender: make(map[string][]*Transaction),
	}
}

// Add inserts a transaction into the pool. It returns an error if the
// transaction is a duplicate. If the pool is at capacity, the lowest
// gas-price transaction is evicted provided the new one has a higher price.
func (p *Pool) Add(tx *Transaction) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Reject duplicates.
	if _, exists := p.byHash[tx.Hash]; exists {
		return fmt.Errorf("transaction %s already in pool", tx.Hash)
	}

	// Reject duplicate nonce from same sender.
	for _, existing := range p.bySender[tx.Sender] {
		if existing.Nonce == tx.Nonce {
			return fmt.Errorf("sender %s already has a transaction with nonce %d", tx.Sender, tx.Nonce)
		}
	}

	// Evict if at capacity.
	if p.pq.Len() >= p.config.MaxSize {
		lowest := p.pq.RemoveLowest()
		if lowest != nil {
			if tx.GasPrice <= lowest.GasPrice {
				// Put it back; new tx doesn't beat it.
				p.pq.Push(lowest)
				return fmt.Errorf("pool is full and transaction gas price %d does not exceed floor %d", tx.GasPrice, lowest.GasPrice)
			}

			// Avoid manufacturing a nonce gap: if the cheapest tx has a
			// higher-nonce sibling still in the pool, evicting it would leave
			// an unprocessable gap. In that case evict the sender's
			// highest-nonce tx instead (dropping the tail is gap-free) and put
			// the cheapest tx back.
			victim := lowest
			if siblings := p.bySender[lowest.Sender]; len(siblings) > 1 &&
				siblings[len(siblings)-1].Nonce > lowest.Nonce {
				tail := siblings[len(siblings)-1]
				if tail.Hash != lowest.Hash {
					p.pq.Push(lowest)
					p.pq.RemoveByHash(tail.Hash)
					victim = tail
				}
			}

			// Evict the chosen victim.
			delete(p.byHash, victim.Hash)
			p.removeSenderTx(victim)
		}
	}

	p.pq.Push(tx)
	p.byHash[tx.Hash] = tx
	p.bySender[tx.Sender] = append(p.bySender[tx.Sender], tx)
	// Keep per-sender list sorted by nonce.
	sort.Slice(p.bySender[tx.Sender], func(i, j int) bool {
		return p.bySender[tx.Sender][i].Nonce < p.bySender[tx.Sender][j].Nonce
	})

	return nil
}

// removeSenderTx removes a specific transaction from the per-sender index.
func (p *Pool) removeSenderTx(tx *Transaction) {
	txs := p.bySender[tx.Sender]
	for i, t := range txs {
		if t.Hash == tx.Hash {
			p.bySender[tx.Sender] = append(txs[:i], txs[i+1:]...)
			break
		}
	}
	if len(p.bySender[tx.Sender]) == 0 {
		delete(p.bySender, tx.Sender)
	}
}

// PendingByAddress returns all pending transactions for a given sender, sorted by nonce.
func (p *Pool) PendingByAddress(sender string) []*Transaction {
	p.mu.RLock()
	defer p.mu.RUnlock()

	txs := p.bySender[sender]
	if txs == nil {
		return nil
	}
	result := make([]*Transaction, len(txs))
	copy(result, txs)
	return result
}

// DetectNonceGaps scans all senders and reports any gaps in nonce sequences.
func (p *Pool) DetectNonceGaps() []NonceGap {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var gaps []NonceGap
	for sender, txs := range p.bySender {
		if len(txs) < 2 {
			continue
		}
		for i := 1; i < len(txs); i++ {
			expected := txs[i-1].Nonce + 1
			if txs[i].Nonce != expected {
				gaps = append(gaps, NonceGap{
					Sender:   sender,
					Expected: expected,
					Found:    txs[i].Nonce,
				})
			}
		}
	}
	return gaps
}

// Status returns a snapshot of the pool state.
func (p *Pool) Status() PoolStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status := PoolStatus{
		Size:        p.pq.Len(),
		MaxSize:     p.config.MaxSize,
		SenderCount: len(p.bySender),
	}

	if top := p.pq.Peek(); top != nil {
		status.TopGasPrice = top.GasPrice
	}

	// Find floor gas price.
	if p.pq.Len() > 0 {
		all := p.pq.All()
		minPrice := all[0].GasPrice
		for _, tx := range all[1:] {
			if tx.GasPrice < minPrice {
				minPrice = tx.GasPrice
			}
		}
		status.FloorGasPrice = minPrice
	}

	// Include nonce gaps in status.
	// Release read lock temporarily to avoid calling DetectNonceGaps which also locks.
	var gaps []NonceGap
	for sender, txs := range p.bySender {
		if len(txs) < 2 {
			continue
		}
		for i := 1; i < len(txs); i++ {
			expected := txs[i-1].Nonce + 1
			if txs[i].Nonce != expected {
				gaps = append(gaps, NonceGap{
					Sender:   sender,
					Expected: expected,
					Found:    txs[i].Nonce,
				})
			}
		}
	}
	status.NonceGaps = gaps

	return status
}
