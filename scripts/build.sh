#!/usr/bin/env bash
# 构建主机部署发布包：编译后端二进制 + 构建前端静态文件，汇总到 release/
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT="$ROOT/release"
GOOS_TARGET="${GOOS:-linux}"
GOARCH_TARGET="${GOARCH:-amd64}"

echo ">> 清理输出目录 $OUT"
rm -rf "$OUT" && mkdir -p "$OUT"

echo ">> 构建后端二进制 ($GOOS_TARGET/$GOARCH_TARGET)..."
cd "$ROOT/backend"
CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
  go build -ldflags="-s -w" -o "$OUT/cmdb-server" ./cmd/server

echo ">> 构建前端..."
cd "$ROOT/frontend"
npm install
npm run build
cp -r dist "$OUT/frontend"

echo ">> 拷贝部署文件..."
cp "$ROOT/deploy/host/config.example.yaml" "$OUT/config.yaml"
cp "$ROOT/deploy/host/cmdb.service" "$OUT/"
cp "$ROOT/deploy/host/nginx.conf" "$OUT/"
cp "$ROOT/deploy/host/install.sh" "$OUT/"
chmod +x "$OUT/install.sh"

echo ">> 完成，发布包位于：$OUT"
ls -lh "$OUT"
