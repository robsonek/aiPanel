.PHONY: build dev test test-fe lint clean

## Build production binary (frontend + Go)
build:
	cd web && pnpm build
	CGO_ENABLED=0 go build -o bin/aipanel ./cmd/aipanel

## Start development environment (backend + Vite dev server)
dev:
	@./scripts/dev.sh

## Run Go tests
test:
	go test ./...

## Run frontend tests
test-fe:
	cd web && pnpm test

## Run linters (Go + frontend)
lint:
	golangci-lint run ./...
	cd web && pnpm lint

## Remove build artifacts
clean:
	rm -rf bin/ web/dist/
