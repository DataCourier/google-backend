.PHONY: setup sync deploy deploy-user deploy-game deploy-org deploy-stock test clean help emulator dev-game

# Load config
PROJECT_NAME ?= my-app
GCP_REGION ?= us-central1

help:
	@echo "Available commands:"
	@echo ""
	@echo "Local Development:"
	@echo "  make emulator    - Start Firestore emulator (keep running in terminal)"
	@echo "  make dev-game    - Run game-service locally (requires emulator running)"
	@echo ""
	@echo "Setup & Deploy:"
	@echo "  make setup       - Run initial setup (sync shared code, update modules)"
	@echo "  make sync        - Sync /shared code to all services"
	@echo "  make deploy      - Deploy all services to Cloud Run"
	@echo "  make deploy-user - Deploy user-service only"
	@echo "  make deploy-game - Deploy game-service only"
	@echo "  make deploy-org  - Deploy org-service only"
	@echo "  make deploy-stock- Deploy stock-prices service"
	@echo ""
	@echo "Other:"
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
		--set-env-vars ENV=production,GCP_PROJECT=michal-playground-2026 \
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

deploy-stock:
	@echo "Deploying stock-prices..."
	@cd standalone-apis/stock-prices && \
		gcloud run deploy stock-prices \
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

# Local Development
emulator:
	@echo "🔥 Starting Firestore emulator..."
	@echo "Keep this terminal open! Press Ctrl+C to stop."
	@gcloud emulators firestore start

dev-game:
	@echo "🎮 Starting game-service locally..."
	@echo ""
	@echo "Make sure emulator is running in another terminal:"
	@echo "  make emulator"
	@echo ""
	@cd services/game-service && \
		export PATH=$$HOME/go/bin:$$PATH && \
		export GCP_PROJECT=michal-playground-2026 && \
		export FIRESTORE_EMULATOR_HOST=localhost:8080 && \
		go run .
