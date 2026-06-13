GO_BIN := $(shell go env GOPATH)/bin
GOOSE ?= $(GO_BIN)/goose
SQLC ?= $(GO_BIN)/sqlc
DATABASE_URL ?= postgres://clipanvil:clipanvil_dev@localhost:5432/clipanvil?sslmode=disable

.PHONY: server-dev server-build server-test server-lint migrate migrate-up migrate-down migrate-create sqlc-generate

server-dev:
	cd apps/server && go run ./cmd/server

server-build:
	cd apps/server && go build -o ../../bin/server ./cmd/server

server-test:
	cd apps/server && go test ./...

server-lint:
	cd apps/server && golangci-lint run ./...

migrate:
	$(MAKE) migrate-up

migrate-up:
	cd apps/server && $(GOOSE) -dir migrations postgres "$(DATABASE_URL)" up

migrate-down:
	cd apps/server && $(GOOSE) -dir migrations postgres "$(DATABASE_URL)" down

migrate-create:
	cd apps/server && $(GOOSE) -dir migrations create $(name) sql

sqlc-generate:
	cd apps/server && $(SQLC) generate
