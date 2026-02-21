.PHONY: build test lint run docker-up docker-down migrate generate e2e-smoke e2e e2e-run

BINARY := bin/gateway
CMD     := ./cmd/gateway

# Load .env.local if it exists (API keys for local dev, never committed)
-include .env.local
export

build:
	go build -o $(BINARY) $(CMD)

test:
	go test ./...

lint:
	golangci-lint run

run:
	go run $(CMD)

docker-up:
	docker compose -f docker/docker-compose.yml up -d

docker-down:
	docker compose -f docker/docker-compose.yml down

migrate:
	@echo "Usage: goose -dir migrations postgres \"$$DATABASE_URL\" up"

generate:
	sqlc generate

e2e-smoke:
	bash scripts/e2e_smoke.sh

e2e:
	go test -tags e2e -c -o bin/e2e.test.exe ./tests/e2e && \
	GATEWAY_URL=$${GATEWAY_URL:-http://localhost:8080} MASTER_KEY=$${MASTER_KEY:-admin123} \
	./bin/e2e.test.exe -test.v -test.timeout 5m

e2e-run:
	go test -tags e2e -c -o bin/e2e.test.exe ./tests/e2e && \
	GATEWAY_URL=$${GATEWAY_URL:-http://localhost:8080} MASTER_KEY=$${MASTER_KEY:-admin123} \
	./bin/e2e.test.exe -test.v -test.timeout 5m -test.run $(TEST)
