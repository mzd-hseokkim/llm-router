.PHONY: build test lint run docker-up docker-down migrate generate

BINARY := bin/gateway
CMD     := ./cmd/gateway

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
