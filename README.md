# tx-mempool-simulator

[![CI](https://github.com/ai-pavel/tx-mempool-simulator/actions/workflows/ci.yml/badge.svg)](https://github.com/ai-pavel/tx-mempool-simulator/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/ai-pavel/tx-mempool-sim/branch/main/graph/badge.svg)](https://codecov.io/gh/ai-pavel/tx-mempool-sim)

A blockchain transaction mempool simulator written in Go. It provides a JSON-RPC 2.0 HTTP interface for submitting transactions and querying pool state.

## Features

- **Priority queue** ordered by gas price (highest first) with timestamp tie-breaking
- **Configurable max pool size** with automatic eviction of lowest gas-price transactions
- **Nonce-gap detection** per sender address
- **JSON-RPC 2.0** HTTP server with three methods

## Build and Run

```bash
make build
./tx-mempool-simulator -addr :8545 -max-size 5000
```

## JSON-RPC Methods

### sendTransaction

Submit a new transaction to the mempool.

```bash
curl -X POST http://localhost:8545 -H 'Content-Type: application/json' -d '{
  "jsonrpc": "2.0",
  "method": "sendTransaction",
  "params": {"sender": "0xAlice", "nonce": 0, "gasPrice": 50, "size": 200},
  "id": 1
}'
```

### getPoolStatus

Get current mempool status including size, sender count, gas price range, and nonce gaps.

```bash
curl -X POST http://localhost:8545 -H 'Content-Type: application/json' -d '{
  "jsonrpc": "2.0",
  "method": "getPoolStatus",
  "params": {},
  "id": 2
}'
```

### getPendingByAddress

Get all pending transactions for a specific sender address, sorted by nonce.

```bash
curl -X POST http://localhost:8545 -H 'Content-Type: application/json' -d '{
  "jsonrpc": "2.0",
  "method": "getPendingByAddress",
  "params": {"address": "0xAlice"},
  "id": 3
}'
```

## Testing

```bash
make test
```

## Project Structure

```
.
├── main.go                  # Entry point, flag parsing, server startup
├── mempool/
│   ├── transaction.go       # Transaction model with SHA3-256 hashing
│   ├── priority_queue.go    # Heap-based priority queue by gas price
│   ├── pool.go              # Thread-safe mempool with eviction and nonce-gap detection
│   ├── server.go            # JSON-RPC 2.0 HTTP server
│   ├── pool_test.go         # Unit tests for pool logic
│   └── server_test.go       # Integration tests for the RPC server
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Dependencies

- Go 1.21+
- `golang.org/x/crypto` (SHA3-256 hashing)
