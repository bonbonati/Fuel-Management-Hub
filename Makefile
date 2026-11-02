# PetroFleet Sudan — developer entry points. `make help` lists targets.
# Requires: Go (any version; the 1.27.3 toolchain is fetched automatically),
# Node ≥ 24 (pnpm is run through npx), Docker with compose, and kubectl for k8s targets.

SHELL := /bin/bash
GO    := GOTOOLCHAIN=go1.27.3 go
PNPM  := npx -y pnpm@12.16.1
SERVICES := auth fleet station payment fueling
IMAGES   := $(addprefix petro/,$(addsuffix :dev,$(SERVICES) web))

.PHONY: help test test-backend test-frontend lint up down logs e2e e2e-live images k8s-validate k8s-dev k8s-dev-delete

help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n",$$1,$$2}'

test: test-backend test-frontend ## Run all unit tests

test-backend: ## Go: vet + unit tests (race detector) for every service
	cd backend && $(GO) vet ./... && $(GO) test -race -count=1 ./...

test-frontend: ## TS: typecheck + unit tests for shared and web
	cd frontend && $(PNPM) install --frozen-lockfile && $(PNPM) -r typecheck && $(PNPM) -r test

lint: ## gofmt check
	@test -z "$$(gofmt -l backend)" || (gofmt -l backend; echo "run: gofmt -w backend"; exit 1)

up: ## Build and start the local environment (http://localhost:8080)
	docker compose up --build -d

down: ## Stop the local environment (add V=1 to also delete volumes)
	docker compose down $(if $(V),-v,)

logs: ## Follow service logs
	docker compose logs -f auth fleet station payment fueling

e2e: ## End-to-end tests against the running stack (E2E_BASE_URL, default http://localhost:8080)
	cd backend && $(GO) test -count=1 -tags e2e ./e2e/ -v

e2e-live: ## Web UI smoke test against the running stack
	cd frontend && LIVE_GATEWAY=$${LIVE_GATEWAY:-http://localhost:8080} $(PNPM) --filter @petro/web exec vitest run test/live.test.tsx

images: ## Build all images (tag :dev)
	docker compose build

k8s-validate: ## Render both overlays and validate them against the k8s schemas (no cluster needed)
	@for o in dev prod; do kubectl kustomize deploy/k8s/overlays/$$o > /tmp/petro-$$o.yaml && \
	  docker run --rm -v /tmp:/m ghcr.io/yannh/kubeconform:v0.7.0 -strict -summary -kubernetes-version 1.34.0 /m/petro-$$o.yaml; done

k8s-dev: images ## Build images, import them into k3s and apply the dev overlay
	docker save $(IMAGES) | sudo k3s ctr images import -
	kubectl apply -k deploy/k8s/overlays/dev
	kubectl -n petro-dev rollout status deploy --timeout=300s

k8s-dev-delete: ## Remove the dev deployment (namespace petro-dev, including its volumes)
	kubectl delete -k deploy/k8s/overlays/dev
