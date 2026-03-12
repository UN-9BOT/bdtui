.PHONY: test build test-db init-test-db

test:
	go test ./...

build:
	go build ./...
	mkdir -p bin
	go build -o bin/bdtui ./cmd/bdtui

TEST_DB_DIR := $(CURDIR)/tests/fixtures/testdb

init-test-db:
	@if [ ! -d "$(TEST_DB_DIR)/.beads/dolt" ]; then \
		echo "Initializing test database..."; \
		cd $(TEST_DB_DIR) && bd init --from-jsonl --prefix test; \
	else \
		echo "Test database already initialized."; \
	fi

test-db: build init-test-db
	./bin/bdtui --beads-dir $(TEST_DB_DIR)/.beads
