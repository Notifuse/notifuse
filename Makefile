.PHONY: build test-unit run clean keygen test-service test-repo test-http test-migrations test-database test-pkg dev coverage docker-build docker-run docker-stop docker-clean docker-logs docker-buildx-setup docker-publish docker-compose-up docker-compose-down docker-compose-build

build:
	go build -o bin/server ./cmd/api

test-unit:
	go test -race -v ./internal/domain  ./internal/http ./internal/service ./internal/service/broadcast ./internal/repository ./internal/migrations ./internal/database

# End-to-end test command for Cursor Agent: runs integration tests only (non-verbose)
e2e-test-within-cursor-agent:
	@echo "Running connection pool integration tests (non-verbose)..."
	@./run-integration-tests.sh "TestConnectionPoolLifecycle$$" 2>&1 | grep -E "PASS|FAIL|^ok|===|^---" || true
	@sleep 3
	@./run-integration-tests.sh "TestConnectionPoolConcurrency$$" 2>&1 | grep -E "PASS|FAIL|^ok|===|^---" || true
	@sleep 3
	@./run-integration-tests.sh "TestConnectionPoolLimits$$" 2>&1 | grep -E "PASS|FAIL|^ok|===|^---" || true
	@sleep 3
	@./run-integration-tests.sh "TestConnectionPoolFailureRecovery$$" 2>&1 | grep -E "PASS|FAIL|^ok|===|^---" || true
	@sleep 3
	@./run-integration-tests.sh "TestConnectionPoolPerformance$$" 2>&1 | grep -E "PASS|FAIL|^ok|===|^---" || true
	@echo "\n✅ All integration tests completed"

test-integration:
	INTEGRATION_TESTS=true go test -race -timeout 9m ./tests/integration/ -v

test-domain:
	go test -race -v ./internal/domain

test-service:
	go test -race -v ./internal/service ./internal/service/broadcast

test-repo:
	go test -race -v ./internal/repository

test-http:
	go test -race -v ./internal/http

test-migrations:
	go test -race -v ./internal/migrations

test-database:
	go test -race -v ./internal/database ./internal/database/schema

test-pkg:
	go test -race -v ./pkg/...

# Connection pool test commands
# Note: Run these individually to avoid connection exhaustion in CI
test-connection-pools:
	@echo "Running connection pool tests (individually to avoid connection exhaustion)..."
	@./run-integration-tests.sh "TestConnectionPoolLifecycle$$"
	@sleep 3
	@./run-integration-tests.sh "TestConnectionPoolConcurrency$$"
	@sleep 3
	@./run-integration-tests.sh "TestConnectionPoolLimits$$"
	@sleep 3
	@./run-integration-tests.sh "TestConnectionPoolFailureRecovery$$"
	@sleep 3
	@./run-integration-tests.sh "TestConnectionPoolPerformance$$"

test-connection-pools-race:
	@echo "Running connection pool tests with race detector (individually)..."
	@GOFLAGS="-race" ./run-integration-tests.sh "TestConnectionPoolLifecycle$$"
	@sleep 3
	@GOFLAGS="-race" ./run-integration-tests.sh "TestConnectionPoolConcurrency$$"
	@sleep 3
	@GOFLAGS="-race" ./run-integration-tests.sh "TestConnectionPoolLimits$$"
	@sleep 3
	@GOFLAGS="-race" ./run-integration-tests.sh "TestConnectionPoolFailureRecovery$$"

test-connection-pools-short:
	@echo "Running fast connection pool tests only..."
	@./run-integration-tests.sh "TestConnectionPoolLifecycle$$"
	@sleep 2
	@./run-integration-tests.sh "TestConnectionPoolLimits$$"

test-connection-pools-leak-check:
	@echo "Running connection pool tests with leak detection..."
	@./run-integration-tests.sh "TestConnectionPoolLifecycle$$"
	@echo "Checking for leaked connections..."
	@docker exec tests-postgres-test-1 psql -U notifuse_test -d postgres -c \
		"SELECT COUNT(*) as leaked_connections FROM pg_stat_activity WHERE usename = 'notifuse_test' AND state != 'idle' AND pid != pg_backend_pid();" 2>/dev/null || \
		echo "Note: PostgreSQL connection check skipped (database may not be available)"

# Comprehensive test coverage command
coverage:
	@echo "Running comprehensive tests and generating coverage report..."
	@go test -race -coverprofile=coverage.out -covermode=atomic $$(go list ./... | grep -v '/tests/integration') -v
	@echo "\n=== Comprehensive Test Coverage Summary ==="
	@go tool cover -func=coverage.out | grep total
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Detailed HTML coverage report generated: coverage.html"

run:
	go run ./cmd/api

dev:
	air

clean:
	rm -rf bin/ tmp/ coverage.out coverage.html

keygen:
	go run cmd/keygen/main.go

# Docker commands
docker-build:
	@echo "Building Docker image..."
	docker build -t notifuse:latest .

docker-run:
	@echo "Running Docker container..."
	docker run -d --name notifuse \
		-p 8080:8080 \
		-e PASETO_PRIVATE_KEY=$${PASETO_PRIVATE_KEY} \
		-e PASETO_PUBLIC_KEY=$${PASETO_PUBLIC_KEY} \
		-e ROOT_EMAIL=$${ROOT_EMAIL:-admin@example.com} \
		-e API_ENDPOINT=$${API_ENDPOINT:-http://localhost:8080} \
		-e WEBHOOK_ENDPOINT=$${WEBHOOK_ENDPOINT:-http://localhost:8080} \
		notifuse:latest

docker-stop:
	@echo "Stopping Docker container..."
	docker stop notifuse || true
	docker rm notifuse || true

docker-clean: docker-stop
	@echo "Removing Docker image..."
	docker rmi notifuse:latest || true

docker-logs:
	@echo "Showing Docker container logs..."
	docker logs -f notifuse

docker-buildx-setup:
	@echo "Setting up Docker buildx for multi-platform builds..."
	@docker buildx create --name notifuse-builder --use --bootstrap 2>/dev/null || \
		docker buildx use notifuse-builder 2>/dev/null || \
		echo "Buildx builder already exists and is active"
	@docker buildx inspect --bootstrap

docker-publish:
	@echo "Building and publishing multi-platform Docker image to Docker Hub..."
	@if [ -z "$(word 2,$(MAKECMDGOALS))" ]; then \
		echo "Building with tag: latest for amd64 and arm64"; \
		docker buildx build --platform linux/amd64,linux/arm64 -t notifuse/notifuse:latest --push .; \
	else \
		echo "Building with tag: $(word 2,$(MAKECMDGOALS)) for amd64 and arm64"; \
		docker buildx build --platform linux/amd64,linux/arm64 -t notifuse/notifuse:$(word 2,$(MAKECMDGOALS)) --push .; \
	fi

# This prevents make from trying to run the tag as a target
%:
	@:

# Docker compose commands
docker-compose-up:
	@echo "Starting services with Docker Compose..."
	docker-compose up -d

docker-compose-down:
	@echo "Stopping services with Docker Compose..."
	docker-compose down

docker-compose-build:
	@echo "Building services with Docker Compose..."
	docker-compose build

.DEFAULT_GOAL := build 