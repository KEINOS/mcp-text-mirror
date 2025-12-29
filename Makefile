# Phony targets: do not correspond to files
.PHONY: all build test lint fmt clean

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
	golangci-lint run --fix

# Run linters first, then run tests with race detector and coverage
test: lint
	@echo "Running tests with race detector and coverage..."
	go test -cover -race ./...

# Remove build artifacts
clean:
	rm -f text-mirror
	rm -f ./*.out
	@echo "cleaned"
