#!/usr/bin/env bash
# 开发辅助：下载 rathole v0.5.0 到 bin/（正式分发见 PRD D-1 自构建计划）
set -euo pipefail
VER="v0.5.0"
REPO="rathole-org/rathole"
OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS/$ARCH" in
  Linux/x86_64)  A="rathole-x86_64-unknown-linux-gnu.zip" ;;
  Darwin/arm64)  A="rathole-aarch64-apple-darwin.zip" ;;
  Darwin/x86_64) A="rathole-x86_64-apple-darwin.zip" ;;
  *) echo "平台 $OS/$ARCH 请手动从 https://github.com/$REPO/releases 下载" >&2; exit 1 ;;
esac
mkdir -p bin
echo "下载 $REPO $VER ($A)…"
gh release download "$VER" --repo "$REPO" --pattern "$A" --clobber -O bin/rathole.zip
unzip -o -q bin/rathole.zip -d bin/
rm -f bin/rathole.zip
chmod +x bin/rathole 2>/dev/null || true
echo "完成：bin/rathole（不内嵌：HackTool 专杀特征会连带标记宿主，D-13）"
