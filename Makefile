.PHONY: build test bench clean proto fmt lint vet deps

# Build the project
build:
	go build ./...

# Run tests
test:
	go test -v ./...

# Run integration tests (requires running Nova Cache server)
test-integration:
	go test -v -run TestIntegrationLocalServer ./...

# Run all tests including integration
test-all: test test-integration

# Run benchmarks
bench:
	go test -bench=. -benchmem ./...

# Clean build artifacts
clean:
	go clean ./...
	rm -f coverage.out

# Generate protobuf code
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/cache.proto

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	golangci-lint run

# Run vet
vet:
	go vet ./...

# Install dependencies
deps:
	go mod download
	go mod tidy

# Install dev tools
dev-deps:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run coverage
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

# Run all checks
check: fmt vet lint test

# Build example
example:
	cd examples && go build -o example main.go

# Run example (requires running Nova Cache server)
run-example:
	cd examples && go run main.go

# Generate documentation
docs:
	godoc -http=:6060

# Release preparation
release: clean check coverage
	@echo "Ready for release"
