.PHONY: build test lint integration-test mocks clean check-cgo

CGO_ENABLED ?= 1
BINARY_NAME  = warpscope
BUILD_DIR    = ./bin

# Ensure CGO is available (required for BLS12-381 / blst)
check-cgo:
	@which gcc > /dev/null || (echo "ERROR: gcc not found. CGO_ENABLED=1 is required for BLS (blst)." && exit 1)

build: check-cgo
	CGO_ENABLED=$(CGO_ENABLED) go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/warpscope

test: check-cgo
	CGO_ENABLED=$(CGO_ENABLED) go test ./internal/... -v -count=1

integration-test: check-cgo
	CGO_ENABLED=$(CGO_ENABLED) go test -tags integration ./test/integration/... -v -timeout 120s

lint:
	golangci-lint run ./...

mocks:
	go generate ./...

clean:
	rm -rf $(BUILD_DIR)

tidy:
	go mod tidy

all: build test
