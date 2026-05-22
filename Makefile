BINARY := restore-drill
MODULE := github.com/fluentorbit/restore-drill
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test lint clean release docker

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/restore-drill

test:
	go test -race -count=1 ./...

test-integration:
	go test -race -count=1 -tags=integration ./...

lint:
	golangci-lint run ./...

fmt:
	gofumpt -w .
	goimports -w .

clean:
	rm -rf bin/ dist/

docker:
	docker build -t $(BINARY):$(VERSION) .

release:
	goreleaser release --clean

snapshot:
	goreleaser release --snapshot --clean

.DEFAULT_GOAL := build
