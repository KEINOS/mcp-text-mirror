# Phony targets: do not correspond to files
.PHONY: all build test lint fmt fuzz clean

# Default target
all: build

build:
	go build -o text-mirror ./...
	chmod +x text-mirror

# Format the code (in-place)
fmt:
	@gofmt -s -w .
	@echo "go fmt applied"

lint:
	@echo "Running static analysis and linters (golangci-lint)..."
	golangci-lint run --fix

# Run linters first, then run tests with race detector and coverage
test: lint
	@echo "Running tests with race detector and coverage..."
	go test -cover -race ./...

# Run fuzz tests (default: 30 seconds, override with FUZZTIME=1m make fuzz)
# Example: make fuzz FUZZTIME=1m
FUZZTIME ?= 30s
fuzz:
	@echo "Running fuzz tests for $(FUZZTIME)..."
	go test -fuzz=Fuzz -fuzztime=$(FUZZTIME) ./...

# Remove build artifacts
clean:
	rm -f text-mirror
	rm -f ./*.out
	@echo "cleaned"
