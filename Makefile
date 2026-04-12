ROOT    := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
BINARY  := kcompass
GOFLAGS ?=

.PHONY: all build test lint vet fmt cover clean install check

all: lint test build

build:
	cd $(ROOT) && go build $(GOFLAGS) -o $(BINARY) .

install:
	cd $(ROOT) && go install $(GOFLAGS) .

test:
	cd $(ROOT) && go test -race ./...

cover:
	cd $(ROOT) && go test -race -coverprofile=coverage.out ./...
	cd $(ROOT) && go tool cover -func=coverage.out
	@rm -f $(ROOT)/coverage.out

lint:
	cd $(ROOT) && golangci-lint run --timeout=3m ./...

vet:
	cd $(ROOT) && go vet ./...

fmt:
	cd $(ROOT) && gofmt -w .

check: fmt vet lint test
	@echo "All checks passed."

clean:
	rm -f $(ROOT)/$(BINARY) $(ROOT)/coverage.out
