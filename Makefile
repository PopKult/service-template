.PHONY: run run-relay build test test-integration lint vet generate-mocks migrate-up migrate-down docker-build compose-up compose-down

run:
	go run ./cmd/server

run-relay:
	go run ./cmd/outbox-relay

build:
	go build -o bin/server ./cmd/server
	go build -o bin/outbox-relay ./cmd/outbox-relay

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

lint:
	golangci-lint run ./...

vet:
	go vet ./...

generate-mocks:
	go generate ./...

# Requires: DATABASE_URL, e.g.
#   postgres://service-template:dev-only-password@localhost:5432/service-template?sslmode=disable
migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down 1

docker-build:
	docker build -f deployments/docker/Dockerfile -t service-template:local .

compose-up:
	docker compose -f deployments/docker-compose.yml up --build

compose-down:
	docker compose -f deployments/docker-compose.yml down -v
