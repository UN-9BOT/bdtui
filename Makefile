.PHONY: test build test-db

test:
	go test ./...

build:
	go build ./...
	mkdir -p bin
	go build -o bin/bdtui ./cmd/bdtui

test-db:
	./bin/bdtui --beads-dir $(CURDIR)/tests/fixtures/testdb/.beads
