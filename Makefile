BINARY := restore-drill
MODULE := github.com/fluentorbit/restore-drill
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.Date=$(DATE)

.PHONY: build test lint clean release docker

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/restore-drill

test:
	go test -race -count=1 ./...

test-integration:
	RESTORE_DRILL_INTEGRATION=1 go test -race -count=1 -timeout=10m ./test/integration/...

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
