BINARY := restore-drill
MODULE := github.com/RamazanKara/restore-drill
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.Date=$(DATE)

.PHONY: build test test-unit test-integration test-k8s vet lint vuln fmt clean release snapshot docker docker-smoke verify helm-lint goreleaser-check check-examples cover

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/restore-drill

test: test-unit

test-unit:
	go test -race -count=1 ./...

cover:
	go test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out -o coverage.html

test-integration:
	RESTORE_DRILL_INTEGRATION=1 go test -race -count=1 -timeout=20m ./test/integration/...

test-k8s:
	bash ./test/k8s/smoke.sh

vet:
	go vet ./...

lint:
	golangci-lint run ./...

vuln:
	bash ./scripts/govulncheck.sh

fmt:
	gofumpt -w .
	goimports -w .

check-examples:
	for f in examples/*.yaml; do go run ./cmd/restore-drill validate --config "$$f"; done

helm-lint:
	helm lint deploy/helm/restore-drill --set-file config.inline=examples/redis-rdb.yaml
	helm template restore-drill deploy/helm/restore-drill --set-file config.inline=examples/redis-rdb.yaml >/dev/null
	helm template restore-drill deploy/helm/restore-drill --set-file config.inline=examples/redis-rdb.yaml -f test/k8s/helm-runtime-options.yaml >/dev/null

goreleaser-check:
	goreleaser check

verify: build vet test-unit lint vuln check-examples helm-lint goreleaser-check

clean:
	rm -rf bin/ dist/

docker:
	docker build -t $(BINARY):$(VERSION) .

docker-smoke:
	docker build -t $(BINARY):smoke .
	docker run --rm $(BINARY):smoke version

release:
	goreleaser release --clean

snapshot:
	goreleaser release --snapshot --clean

.DEFAULT_GOAL := build
