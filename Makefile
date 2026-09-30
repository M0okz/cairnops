.DEFAULT_GOAL := help

.PHONY: help bootstrap build test check dev compose-up compose-down

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "; printf "CairnOps development commands\n\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

bootstrap: ## Install Web dependencies and download Go modules
	go mod download
	npm --prefix web install

build: ## Build the Web application and the three Go processes
	npm --prefix web run build
	mkdir -p bin
	go build -o bin/cairnops-server ./cmd/cairnops-server
	go build -o bin/cairnops-worker ./cmd/cairnops-worker
	go build -o bin/cairnops-push-relay ./cmd/cairnops-push-relay

test: ## Run Go tests
	@$(MAKE) --no-print-directory warn-integration
	go test ./...

check: ## Run the same tests and static checks as the CI pipeline
	@$(MAKE) --no-print-directory warn-integration
	npm --prefix web run check
	npm --prefix web test
	npm --prefix web run build
	go test ./...
	go vet ./...

# Les tests d'intégration se sautent en silence sans base : une suite verte ne
# prouve rien tant que CAIRNOPS_TEST_DATABASE_URL n'est pas renseignée. La CI la
# renseigne toujours ; en local, il faut le dire. Voir CONTRIBUTING.md.
.PHONY: warn-integration
warn-integration:
	@if [ -z "$$CAIRNOPS_TEST_DATABASE_URL" ]; then \
		printf '\033[33m'; \
		echo "ATTENTION : CAIRNOPS_TEST_DATABASE_URL n'est pas renseignée."; \
		echo "            Les tests d'intégration vont se sauter en silence :"; \
		echo "            cette suite ne prouvera pas ce qui touche à PostgreSQL."; \
		echo "            Procédure dans CONTRIBUTING.md, section « Lancer les tests »."; \
		printf '\033[0m'; \
	fi

dev: ## Run the API server against a local PostgreSQL instance
	CAIRNOPS_MASTER_KEY_FILE=$${CAIRNOPS_MASTER_KEY_FILE:-tmp/master.key} CAIRNOPS_WEB_DIR=web/build go run ./cmd/cairnops-server

compose-up: ## Build and start the complete local stack
	docker compose up --build

compose-down: ## Stop the local stack
	docker compose down
