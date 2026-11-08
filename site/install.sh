#!/bin/sh
# tunnelport 安装脚本（一键部署 / 一键连接）
#
#   VPS 入口（serve）：  curl -fsSL https://haochan1996.github.io/tunnelport/install.sh | sh
#   桌面连接（connect）：curl -fsSL https://haochan1996.github.io/tunnelport/install.sh | sh -s -- connect tunnelport://<配对链接>
#
# 兼容：Linux x86_64/arm64（systemd 或无 systemd）、macOS x86_64/arm64（launchd）。
# Windows：Releases 直接下载 tp.exe，或在 WSL 中运行本脚本。
# 环境变量：TP_VERSION（默认 v0.1.0）、TP_DOWNLOAD_URL（自定义下载源）、TP_PORT_BASE
set -eu

VERSION="${TP_VERSION:-v0.1.0}"
BASE_URL="https://github.com/haochan1996/tunnelport/releases/download/${VERSION}"

ROLE="serve"
CONNECT_URL=""
if [ "${1:-}" = "connect" ]; then
  ROLE="connect"
  CONNECT_URL="${2:-}"
  if [ -z "$CONNECT_URL" ]; then
    echo "✗ 连接模式需要配对链接：sh -s -- connect tunnelport://…"; exit 1
  fi
fi

OS="$(uname -s)"
ARCH="$(uname -m)"
case "${OS}-${ARCH}" in
  Linux-x86_64)              ASSET="tp_linux_amd64" ;;
  Linux-aarch64|Linux-arm64) ASSET="tp_linux_arm64" ;;
  Darwin-x86_64)             ASSET="tp_darwin_amd64" ;;
  Darwin-arm64)              ASSET="tp_darwin_arm64" ;;
  *)
    echo "✗ 暂不支持：${OS} / ${ARCH}"
    echo "  Windows 请从 GitHub Releases 下载 tp_windows_amd64.exe；或在 WSL 中重跑本脚本"
    exit 1 ;;
esac

echo "==> [1/4] 下载 tunnelport ${VERSION}（${ASSET}）"
DL="${TP_DOWNLOAD_URL:-${BASE_URL}/${ASSET}}"
TMP="${TMPDIR:-/tmp}/tp-install-$$"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL --connect-timeout 10 --max-time 120 "$DL" -o "$TMP" 2>/dev/null \
    || { echo "    主源较慢，切换国内镜像…"; curl -fsSL --connect-timeout 10 --max-time 180 "https://gh-proxy.com/${DL}" -o "$TMP" 2>/dev/null; } \
    || { echo "✗ 下载失败（主源+镜像均不通）。请手动下载：${DL}"; exit 1; }
elif command -v wget >/dev/null 2>&1; then
  wget -q -T 10 -t 2 --timeout=120 -O "$TMP" "$DL" \
    || { echo "    主源较慢，切换国内镜像…"; wget -q -T 10 -t 2 --timeout=180 -O "$TMP" "https://gh-proxy.com/${DL}"; } \
    || { echo "✗ 下载失败（主源+镜像均不通）。请手动下载：${DL}"; exit 1; }
else
  echo "✗ 需要 curl 或 wget"; exit 1
fi
chmod +x "$TMP"

echo "==> [2/4] 安装到 /usr/local/bin/tp"
if [ -w /usr/local/bin ] 2>/dev/null || [ "$(id -u)" = "0" ]; then
  mv "$TMP" /usr/local/bin/tp
else
  sudo mv "$TMP" /usr/local/bin/tp
fi

RUN_ARGS=""
[ "$ROLE" = "connect" ] && RUN_ARGS="connect ${CONNECT_URL}"

echo "==> [3/4] 注册开机自启（崩溃自动重启）"
if [ "$OS" = "Darwin" ]; then
  # macOS：launchd 用户代理
  PLIST="$HOME/Library/LaunchAgents/com.tunnelport.tp.plist"
  mkdir -p "$HOME/Library/LaunchAgents"
  cat > "$PLIST" <<PLISTEOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.tunnelport.tp</string>
  <key>ProgramArguments</key><array>
    <string>/usr/local/bin/tp</string>
    <string>${ROLE}</string>
PLISTEOF
  [ "$ROLE" = "connect" ] && echo "    <string>${CONNECT_URL}</string>" >> "$PLIST"
  cat >> "$PLIST" <<'PLISTEOF'
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/tmp/tunnelport.log</string>
  <key>StandardErrorPath</key><string>/tmp/tunnelport.log</string>
</dict></plist>
PLISTEOF
  launchctl unload "$PLIST" 2>/dev/null || true
  launchctl load "$PLIST"
elif command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  if [ -w /etc/systemd/system ] 2>/dev/null || [ "$(id -u)" = "0" ]; then SUDO=""; else SUDO="sudo"; fi
  cat > /tmp/tunnelport.service.$$ <<UNIT
[Unit]
Description=tunnelport — Orca 稳定入口
After=network-online.target
Wants=network-online.target
[Service]
ExecStart=/usr/local/bin/tp ${ROLE}${CONNECT_URL:+ ${CONNECT_URL}}
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
UNIT
  $SUDO mv /tmp/tunnelport.service.$$ /etc/systemd/system/tunnelport.service
  $SUDO systemctl daemon-reload
  $SUDO systemctl enable --now tunnelport >/dev/null 2>&1
else
  echo "⚠ 未检测到 systemd/launchd：用后台进程方式启动（重启后需手动重跑）"
  setsid nohup /usr/local/bin/tp ${ROLE} ${CONNECT_URL} > "$HOME/tunnelport.log" 2>&1 < /dev/null &
  SUDO=""
fi

echo "==> [4/4] 启动完成，读取输出…"
sleep 6
echo
echo "════════════════════════════════════════════"
if [ "$ROLE" = "serve" ]; then
  echo " 部署完成！地址 / 配对链接 / 二维码："
else
  echo " 连接状态："
fi
echo "════════════════════════════════════════════"
if [ "$OS" = "Darwin" ]; then
  tail -55 /tmp/tunnelport.log 2>/dev/null || echo "（日志：/tmp/tunnelport.log）"
elif command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  $SUDO journalctl -u tunnelport -n 60 --no-pager | tail -55
else
  tail -55 "$HOME/tunnelport.log"
fi
cat <<'NEXT'

──────────────────────────────────────────────────
下一步（共约 3 分钟）：

 serve 模式：① 防火墙放行上面提示的端口段
            ② 入口地址粘贴到 Orca 桌面端「自定义网络地址」
            ③ 桌面机运行 tp connect <配对链接>
 connect 模式：地址粘贴进 Orca → Set up Mobile → 手机扫码

 有问题：tp doctor 会告诉你卡在哪一环。
──────────────────────────────────────────────────
NEXT
