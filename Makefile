.PHONY: build
build:
	@echo "build server and worker"
	@go build -o bin/server ./cmd/server
	@go build -o bin/worker ./cmd/worker

.PHONY: run
run:
	@echo "run server"
	@go run ./cmd/server

.PHONY: run-worker
run-worker:
	@echo "run worker"
	@go run ./cmd/worker

.PHONY: test
test:
	@echo "test"
	@go test ./... -count=1

.PHONY: cover
cover:
	@echo "coverage"
	@go test -count=1 -coverprofile=coverage.out \
		-coverpkg=$(shell go list ./... | grep -v -e /mocks -e /migrations | paste -sd,) \
		$(shell go list ./... | grep -v -e /mocks -e /migrations)
	@go tool cover -func=coverage.out | grep "^total:"

.PHONY: generate
generate:
	@echo "generate mocks"
	@go generate ./...

.PHONY: lint
lint:
	@echo "lint"
	@golangci-lint run ./...

.PHONY: fmt
fmt:
	@echo "fmt"
	@golangci-lint fmt

.PHONY: vet
vet:
	@echo "vet"
	@go vet ./...

# golangci-lint должен быть собран тем же Go, что указан в go.mod:
# go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
.PHONY: install-lint
install-lint:
	@echo "install golangci-lint"
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

# Поднимает только инфраструктуру: server и worker запускаются локально.
.PHONY: infra-up
infra-up:
	@docker compose -f deployments/docker-compose.infra.yaml up -d

.PHONY: infra-down
infra-down:
	@docker compose -f deployments/docker-compose.infra.yaml down

# Поднимает все окружение целиком, включая server и worker в контейнерах.
.PHONY: up
up:
	@docker compose -f deployments/docker-compose.yaml up -d --build

.PHONY: down
down:
	@docker compose -f deployments/docker-compose.yaml down

# --- Kubernetes ---

# Локальный кластер: ingress-контроллер и metrics-server нужны Ingress и HPA.
.PHONY: k8s-up
k8s-up:
	@minikube start --driver=docker --cpus=4 --memory=8192
	@minikube addons enable ingress
	@minikube addons enable metrics-server

.PHONY: k8s-down
k8s-down:
	@minikube delete

.PHONY: k8s-image
k8s-image:
	@eval $$(minikube -p minikube docker-env) && docker build -t gophprofile:latest .

.PHONY: monitoring-up
monitoring-up:
	@helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
	@helm repo update
	@helm upgrade --install monitoring prometheus-community/kube-prometheus-stack \
		--namespace monitoring --create-namespace \
		--set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
		--set alertmanager.enabled=false --wait --timeout 10m

.PHONY: helm-lint
helm-lint:
	@helm lint deployments/helm/gophprofile
	@helm template gophprofile deployments/helm/gophprofile -f deployments/helm/gophprofile/values-dev.yaml >/dev/null

.PHONY: deploy
deploy: k8s-image
	@helm upgrade --install gophprofile deployments/helm/gophprofile \
		-f deployments/helm/gophprofile/values-dev.yaml --wait --timeout 8m

.PHONY: undeploy
undeploy:
	@helm uninstall gophprofile
