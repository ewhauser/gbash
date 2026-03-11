PROJECT ?= $(shell gcloud config get-value project 2>/dev/null)
LOCATION ?= us-west1
MODEL ?= gemini-2.5-flash

.PHONY: run run-vertex-bash-chat
run: run-vertex-bash-chat

run-vertex-bash-chat:
	@if ! command -v gcloud >/dev/null 2>&1; then \
		echo "gcloud is required for make run"; \
		exit 1; \
	fi
	@if [ -z "$(PROJECT)" ]; then \
		echo "No gcloud project is configured. Run: gcloud config set project YOUR_PROJECT_ID"; \
		exit 1; \
	fi
	@if ! gcloud auth application-default print-access-token >/dev/null 2>&1; then \
		echo "No Application Default Credentials found. Running gcloud auth application-default login..."; \
		gcloud auth application-default login; \
	fi
	@echo "Running adk-bash-chat with project $(PROJECT) in $(LOCATION)"
	@GOOGLE_CLOUD_PROJECT="$(PROJECT)" \
	GOOGLE_CLOUD_LOCATION="$(LOCATION)" \
	go run ./adk-bash-chat --backend=vertex --model="$(MODEL)"
