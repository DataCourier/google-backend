.PHONY: setup sync deploy deploy-user deploy-game deploy-org test clean help

# Load config
PROJECT_NAME ?= my-app
GCP_REGION ?= us-central1

help:
	@echo "Available commands:"
	@echo "  make setup       - Run initial setup (sync shared code, update modules)"
	@echo "  make sync        - Sync /shared code to all services"
	@echo "  make deploy      - Deploy all services to Cloud Run"
	@echo "  make deploy-user - Deploy user-service only"
	@echo "  make deploy-game - Deploy game-service only"
	@echo "  make deploy-org  - Deploy org-service only"
	@echo "  make test        - Run tests for all services"
	@echo "  make clean       - Remove synced files from services"

setup:
	@chmod +x setup.sh
	@./setup.sh

sync:
	@echo "🔄 Syncing shared code..."
	@./setup.sh

deploy: setup
	@echo "🚀 Deploying all services..."
	@$(MAKE) deploy-user
	@$(MAKE) deploy-game

deploy-user:
	@echo "🚀 Deploying user-service..."
	@cd services/user-service && \
		gcloud run deploy user-service \
		--source . \
		--region $(GCP_REGION) \
		--allow-unauthenticated \
		--platform managed \
		--quiet

deploy-game:
	@echo "🚀 Deploying game-service..."
	@cd services/game-service && \
		gcloud run deploy game-service \
		--source . \
		--region $(GCP_REGION) \
		--allow-unauthenticated \
		--platform managed \
		--set-env-vars GCP_PROJECT=$(PROJECT_NAME) \
		--quiet

deploy-org:
	@echo "🚀 Deploying org-service..."
	@cd services/org-service && \
		gcloud run deploy org-service \
		--source . \
		--region $(GCP_REGION) \
		--allow-unauthenticated \
		--platform managed \
		--quiet

test:
	@echo "🧪 Running tests..."
	@for service in services/*; do \
		if [ -d "$$service" ]; then \
			echo "Testing $$(basename $$service)..."; \
			cd $$service && go test -v ./... || exit 1; \
			cd ../..; \
		fi \
	done

clean:
	@echo "🧹 Cleaning synced files..."
	@find services -type d -name "auth" -o -name "middleware" -o -name "response" | xargs rm -rf
	@echo "✅ Clean complete"
