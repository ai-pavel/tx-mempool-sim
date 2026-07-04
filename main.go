package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pkiselyov/tx-mempool-simulator/mempool"
)

func main() {
	addr := flag.String("addr", ":8545", "HTTP listen address")
	maxSize := flag.Int("max-size", 5000, "maximum number of transactions in the mempool")
	flag.Parse()

	cfg := mempool.Config{MaxSize: *maxSize}
	pool := mempool.NewPool(cfg)
	srv := mempool.NewServer(pool)

	fmt.Fprintf(os.Stdout, "tx-mempool-simulator starting on %s (max pool size: %d)\n", *addr, *maxSize)
	log.Fatal(srv.ListenAndServe(*addr))
}
