.PHONY: build test run clean fmt vet

BINARY := tx-mempool-simulator

build:
	go build -o $(BINARY) .

test:
	go test -v -race ./...

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)
	go clean -testcache

fmt:
	go fmt ./...

vet:
	go vet ./...
