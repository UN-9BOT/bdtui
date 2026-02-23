.PHONY: test build

test:
	go test ./...

build:
	go build ./...
	mkdir -p bin
	go build -o bin/bdtui ./cmd/bdtui
