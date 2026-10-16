SHELL := /bin/bash
GO ?= go
COMPOSE ?= docker compose
MIGRATE_BIN := $(GO) run ./cmd/migrate

-include .env
export

.PHONY: setup dev build test test-unit test-integration test-e2e lint format \
        migrate-up migrate-down seed generate openapi docker-up docker-down \
        opa-test frontend-dev frontend-build help

setup: ## Install toolchain deps, tidy modules, install frontend deps
	cp -n .env.example .env || true
	$(GO) mod download
	cd web && npm install

dev: ## Start infra in docker, run API + worker locally
	$(COMPOSE) up -d postgres redis kafka opa openfga openfga-migrate
	$(MAKE) migrate-up
	$(GO) run ./cmd/api

build: ## Build all Go binaries
	$(GO) build ./cmd/...
	$(GO) build -o bin/api ./cmd/api
	$(GO) build -o bin/worker ./cmd/worker
	$(GO) build -o bin/migrate ./cmd/migrate

test: test-unit ## Run all default tests

test-unit: ## Run Go unit tests
	$(GO) test ./cmd/... ./internal/... -count=1

test-integration: ## Run integration tests (requires docker infra up)
	$(GO) test -tags=integration ./tests/integration/... -count=1 -v

test-e2e: ## Run Playwright end-to-end tests (requires full stack up)
	cd web && npx playwright test

lint: ## Run linters
	$(GO) vet ./cmd/... ./internal/...
	cd web && npm run lint

format: ## Format code
	gofmt -w cmd internal tests
	cd web && npm run format

migrate-up: ## Apply all database migrations
	$(MIGRATE_BIN) -direction up

migrate-down: ## Roll back one migration
	$(MIGRATE_BIN) -direction down -steps 1

seed: ## Seed bootstrap admin + FGA tuples (idempotent; also runs on API start)
	$(GO) run ./cmd/api -seed-only

generate: openapi ## Run all code generation

openapi: ## Generate TypeScript API types from the OpenAPI spec
	cd web && npm run generate:api

opa-test: ## Run OPA policy unit tests
	docker run --rm -v $(PWD)/deployments/opa:/policy openpolicyagent/opa:1.4.2 test /policy/policies /policy/tests -v

docker-up: ## Start the full stack
	$(COMPOSE) up --build -d

docker-down: ## Stop the full stack
	$(COMPOSE) down -v

frontend-dev:
	cd web && npm run dev

frontend-build:
	cd web && npm run build

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' Makefile | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
