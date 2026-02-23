.PHONY: test build

test:
	go test ./...

build:
	go build ./...
	go build -o bdtui ./cmd/bdtui
