SHELL := /usr/bin/env bash
.DEFAULT_GOAL := build

BINARY      := oraclewatch
PKG         := github.com/brennocm/oraclewatch
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GOFLAGS     ?= -trimpath
COVER_PKGS  := ./internal/checks/...,./internal/alert/...,./internal/config/...,./internal/reference/...,./internal/feeds/...,./internal/redact/...
FUZZTIME    ?= 30s
SOLC        ?= solc-0.8.37
IMAGE       ?= ghcr.io/brennocm/oraclewatch:dev
COMPOSE     := docker compose -f deploy/docker-compose.yml --env-file .env

.PHONY: build test test-race test-integration test-fork cover lint vuln fuzz abigen contracts docker compose-up compose-down clean

build: ## Build the binary into ./bin
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/oraclewatch

test: ## Unit tests
	go test ./...

test-race: ## Unit tests with the race detector
	go test -race ./...

test-integration: ## Integration tests against the go-ethereum simulated backend
	go test -race -tags integration ./...

test-fork: ## Tests against an Anvil mainnet fork (needs anvil and MAINNET_RPC_URL)
	@test -n "$$MAINNET_RPC_URL" || { echo "MAINNET_RPC_URL is not set"; exit 1; }
	go test -tags fork -count=1 ./internal/feeds/...

cover: ## Coverage of the core packages
	go test -race -tags integration -coverpkg=$(COVER_PKGS) -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1

lint: ## golangci-lint (gosec, errcheck, staticcheck, revive, bodyclose, noctx, contextcheck...)
	golangci-lint run ./...

vuln: ## Known-vulnerability scan of dependencies and stdlib
	govulncheck ./...

fuzz: ## Run every fuzz target for FUZZTIME each
	@set -e; for pkg in $$(go list ./...); do \
	  for f in $$(go test -list '^Fuzz' $$pkg | grep '^Fuzz' || true); do \
	    echo "==> $$pkg $$f"; go test -run '^$$' -fuzz "^$$f$$" -fuzztime $(FUZZTIME) $$pkg; \
	  done; \
	done

abigen: ## Regenerate the test doubles' Go bindings from their committed ABI/bytecode
	scripts/build-contracts.sh --bindings-only

contracts: ## Recompile the Solidity mocks used by integration tests (pinned solc)
	SOLC=$(SOLC) scripts/build-contracts.sh

docker: ## Build the container image for the host platform
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) -t $(IMAGE) .

compose-up: ## Start oraclewatch + Prometheus + Grafana + Caddy
	$(COMPOSE) up -d

compose-down:
	$(COMPOSE) down

clean:
	rm -rf bin dist coverage.out
