.PHONY: server-dev server-build server-test server-lint migrate

server-dev:
	cd apps/server && go run ./cmd/server

server-build:
	cd apps/server && go build -o ../../bin/server ./cmd/server

server-test:
	cd apps/server && go test ./...

server-lint:
	cd apps/server && golangci-lint run ./...

migrate:
	@echo "TODO: implement with goose or golang-migrate in M1+"
