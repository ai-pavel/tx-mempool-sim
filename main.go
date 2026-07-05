package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pavel-ai-agent/tx-mempool-simulator/mempool"
)

func main() {
	addr := flag.String("addr", ":8545", "HTTP listen address")
	maxSize := flag.Int("max-size", 5000, "maximum number of transactions in the mempool")
	maxBody := flag.Int64("max-body-bytes", mempool.DefaultMaxBodyBytes, "maximum accepted request body size in bytes")
	flag.Parse()

	cfg := mempool.Config{MaxSize: *maxSize}
	pool := mempool.NewPool(cfg)
	srv := mempool.NewServer(pool)
	srv.SetMaxBodyBytes(*maxBody)

	fmt.Fprintf(os.Stdout, "tx-mempool-simulator starting on %s (max pool size: %d)\n", *addr, *maxSize)
	log.Fatal(srv.ListenAndServe(*addr))
}
