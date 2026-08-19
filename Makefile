.DEFAULT_GOAL := help

.PHONY: help bootstrap build test check dev compose-up compose-down

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "; printf "CairnOps development commands\n\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

bootstrap: ## Install Web dependencies and download Go modules
	go mod download
	npm --prefix web install

build: ## Build the Web application and both Go processes
	npm --prefix web run build
	mkdir -p bin
	go build -o bin/cairnops-server ./cmd/cairnops-server
	go build -o bin/cairnops-worker ./cmd/cairnops-worker

test: ## Run Go tests
	go test ./...

check: ## Run all static checks and tests
	npm --prefix web run check
	go test ./...
	go vet ./...

dev: ## Run the API server against a local PostgreSQL instance
	CAIRNOPS_MASTER_KEY_FILE=$${CAIRNOPS_MASTER_KEY_FILE:-tmp/master.key} CAIRNOPS_WEB_DIR=web/build go run ./cmd/cairnops-server

compose-up: ## Build and start the complete local stack
	docker compose up --build

compose-down: ## Stop the local stack
	docker compose down
