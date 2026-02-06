#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

cleanup() {
  echo "Stopping dev servers..."
  kill 0 2>/dev/null
}
trap cleanup EXIT

echo "==> Starting Vite dev server..."
cd web && pnpm dev &
VITE_PID=$!
cd ..

echo "==> Starting Go backend..."
go run ./cmd/aipanel &
GO_PID=$!

wait
