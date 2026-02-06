#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
export GOMODCACHE="${PWD}/.cache/gomod"
export GOCACHE="${PWD}/.cache/gobuild"
mkdir -p "${GOMODCACHE}" "${GOCACHE}"

echo "==> Building frontend..."
cd web && pnpm build && cd ..

echo "==> Building Go binary..."
CGO_ENABLED=0 go build -o bin/aipanel ./cmd/aipanel

echo "==> Done: bin/aipanel"
