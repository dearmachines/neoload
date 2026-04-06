default: test

# Build the neoload binary
build:
    go build -C cli -o ../bin/neoload ./cmd/neoload

# Run all tests
test:
    go test -C cli ./...

# Run tests with coverage report (requires 80%+)
cover:
    go test -C cli ./... -coverprofile=../coverage.out
    go tool cover -func=coverage.out | tail -1

# Open HTML coverage report in browser
cover-html: cover
    go tool cover -html=coverage.out

# Run go vet
vet:
    go vet -C cli ./...

# Tidy module dependencies
tidy:
    go mod tidy -C cli

# Remove build artifacts
clean:
    rm -rf bin/ coverage.out
