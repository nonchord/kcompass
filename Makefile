ROOT    := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
BINARY  := kcompass
GOFLAGS ?=

.PHONY: all build test lint vet fmt cover clean install check shellcheck tf-fmt tf-validate tf-check

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

shellcheck:
	cd $(ROOT) && shellcheck *.sh

check: fmt vet lint test shellcheck tf-check
	@echo "All checks passed."

tf-fmt:
	cd $(ROOT) && tofu fmt -recursive terraform/

tf-validate:
	cd $(ROOT)/terraform/examples/basic && tofu init -backend=false -input=false && tofu validate

tf-check: tf-validate
	cd $(ROOT) && tofu fmt -check -recursive terraform/

clean:
	rm -f $(ROOT)/$(BINARY) $(ROOT)/coverage.out
