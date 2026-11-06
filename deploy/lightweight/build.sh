#!/usr/bin/env bash
# 本地构建：Linux amd64 二进制 + 前端 dist 包（Windows 用 Git Bash / WSL 执行）。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="$ROOT/deploy/lightweight/dist"
mkdir -p "$OUT"

echo "==> 后端 linux/amd64"
(cd "$ROOT/server" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w" -o "$OUT/jxc-server" .)

echo "==> 前端 npm ci && npm run build"
(cd "$ROOT/web" && npm ci && npm run build)
tar -czf "$OUT/web-dist.tar.gz" -C "$ROOT/web/dist" .

echo "==> 产物"
ls -lh "$OUT"
