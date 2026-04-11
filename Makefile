.PHONY: test build snapshot

test:
	go test ./...

build:
	go build ./cmd/portico

snapshot:
	goreleaser release --snapshot --clean
