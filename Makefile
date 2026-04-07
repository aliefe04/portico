.PHONY: test build

test:
	go test ./...

build:
	go build ./cmd/portico
