BINARY := restore-drill
EXE := $(shell go env GOEXE)
RACE := $(if $(filter 1,$(shell go env CGO_ENABLED)),-race)
MODULE := github.com/RamazanKara/restore-drill
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.Date=$(DATE)

.PHONY: build test test-unit test-fuzz test-integration test-k8s vet lint staticcheck vuln fmt fmt-check clean release local-release snapshot docker docker-smoke verify helm-lint goreleaser-check check-examples cover docs docs-serve

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)$(EXE) ./cmd/restore-drill

test: test-unit

test-unit:
	go test $(RACE) -count=1 ./...

test-fuzz:
	go test ./internal/config -run='^$$' -fuzz='^FuzzParseConfig$$' -fuzztime=5s -parallel=2
	go test ./internal/config -run='^$$' -fuzz='^FuzzInterpolateEnv$$' -fuzztime=5s -parallel=2
	go test ./internal/cli -run='^$$' -fuzz='^FuzzKeyValueFlags$$' -fuzztime=5s -parallel=2
	go test ./internal/reporter -run='^$$' -fuzz='^FuzzEscapeMarkdown$$' -fuzztime=5s -parallel=2
	go test ./internal/engine -run='^$$' -fuzz='^FuzzExpressionAndValues$$' -fuzztime=5s -parallel=2

cover:
	go test $(RACE) -count=1 -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out -o coverage.html

docs:
	mkdocs build --strict

docs-serve:
	mkdocs serve

test-integration:
	RESTORE_DRILL_INTEGRATION=1 go test $(RACE) -count=1 -timeout=20m ./test/integration/...

test-k8s:
	bash ./test/k8s/smoke.sh

vet:
	go vet ./...

lint:
	golangci-lint run ./...

staticcheck:
	staticcheck ./...

vuln:
	bash ./scripts/govulncheck.sh

fmt:
	gofumpt -w .
	goimports -w .

fmt-check:
	@gofmt -l cmd internal test
	@test -z "$$(gofmt -l cmd internal test)"

check-examples: build
	set -e; for f in examples/*.yaml; do ./bin/$(BINARY)$(EXE) validate --config "$$f"; done

helm-lint:
	helm lint deploy/helm/restore-drill --set-file config.inline=examples/redis-rdb.yaml
	helm template restore-drill deploy/helm/restore-drill --set-file config.inline=examples/redis-rdb.yaml >/dev/null
	helm template restore-drill deploy/helm/restore-drill --set-file config.inline=examples/redis-rdb.yaml -f test/k8s/helm-runtime-options.yaml >/dev/null

goreleaser-check:
	goreleaser check

verify: build fmt-check vet test-unit test-fuzz lint staticcheck vuln check-examples helm-lint goreleaser-check

clean:
	rm -rf bin/ dist/

docker:
	docker build -t $(BINARY):$(VERSION) .

docker-smoke:
	docker build -t $(BINARY):smoke .
	docker run --rm $(BINARY):smoke version

release:
	goreleaser release --clean

local-release:
	CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)" -o dist/$(BINARY)$(EXE) ./cmd/restore-drill
	cd dist && sha256sum $(BINARY)$(EXE) > SHA256SUMS

snapshot:
	goreleaser release --snapshot --clean

.DEFAULT_GOAL := build
