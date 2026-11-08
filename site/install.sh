#!/bin/sh
# tunnelport 一键部署（VPS · Linux）
#
#   curl -fsSL https://haochan1996.github.io/tunnelport/install.sh | sh
#
# 做四件事：下载 → 安装 → 注册开机自启 → 打印地址/配对码/二维码。
# 环境变量：TP_VERSION（默认 v0.1.0）、TP_DOWNLOAD_URL（自定义下载源）、TP_PORT_BASE（默认自动）
set -eu

VERSION="${TP_VERSION:-v0.1.0}"
BASE_URL="https://github.com/haochan1996/tunnelport/releases/download/${VERSION}"

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64)             ASSET="tp_linux_amd64" ;;
  Linux-aarch64|Linux-arm64) ASSET="tp_linux_arm64" ;;
  *)
    echo "✗ 暂不支持的平台: $(uname -s) / $(uname -m)（支持 Linux x86_64、arm64）"
    echo "  macOS / Windows 请从 GitHub Releases 下载对应二进制"
    exit 1 ;;
esac

echo "==> [1/4] 下载 tunnelport ${VERSION}（${ASSET}）"
DL="${TP_DOWNLOAD_URL:-${BASE_URL}/${ASSET}}"
TMP="/tmp/tp-install-$$"
# 国内直连 GitHub 不稳定：主源与镜像都限总时长，避免下载卡死
if command -v curl >/dev/null 2>&1; then
  curl -fsSL --connect-timeout 10 --max-time 120 "$DL" -o "$TMP" 2>/dev/null     || { echo "    主源较慢，切换国内镜像…"; curl -fsSL --connect-timeout 10 --max-time 180 "https://gh-proxy.com/${DL}" -o "$TMP" 2>/dev/null; }     || { echo "✗ 下载失败（主源+镜像均不通）。请手动下载：${DL}"; exit 1; }
elif command -v wget >/dev/null 2>&1; then
  wget -q -T 10 -t 2 --timeout=120 -O "$TMP" "$DL"     || { echo "    主源较慢，切换国内镜像…"; wget -q -T 10 -t 2 --timeout=180 -O "$TMP" "https://gh-proxy.com/${DL}"; }     || { echo "✗ 下载失败（主源+镜像均不通）。请手动下载：${DL}"; exit 1; }
else
  echo "✗ 需要 curl 或 wget"
  exit 1
fi
chmod +x "$TMP"

echo "==> [2/4] 安装到 /usr/local/bin/tp"
if [ -w /usr/local/bin ] 2>/dev/null || [ "$(id -u)" = "0" ]; then
  mv "$TMP" /usr/local/bin/tp
else
  sudo mv "$TMP" /usr/local/bin/tp
fi

echo "==> [3/4] 注册系统服务（开机自启 + 崩溃自动重启）"
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  if [ -w /etc/systemd/system ] 2>/dev/null || [ "$(id -u)" = "0" ]; then SUDO=""; else SUDO="sudo"; fi
  cat <<'UNIT' | $SUDO tee /etc/systemd/system/tunnelport.service >/dev/null
[Unit]
Description=tunnelport — Orca 稳定入口
After=network-online.target
Wants=network-online.target
[Service]
ExecStart=/usr/local/bin/tp serve
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
UNIT
  $SUDO systemctl daemon-reload
  $SUDO systemctl enable --now tunnelport >/dev/null 2>&1
else
  echo "⚠ 未检测到 systemd：用后台进程方式启动（服务器重启后需手动重跑 tp serve）"
  setsid nohup /usr/local/bin/tp serve > "$HOME/tunnelport.log" 2>&1 < /dev/null &
  SUDO=""
fi

echo "==> [4/4] 启动完成，读取地址…"
sleep 6
echo
echo "════════════════════════════════════════════"
echo " 部署完成！下面的地址 / 配对码 / 二维码："
echo "════════════════════════════════════════════"
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  $SUDO journalctl -u tunnelport -n 60 --no-pager | tail -55
else
  tail -55 "$HOME/tunnelport.log"
fi
cat <<'NEXT'

──────────────────────────────────────────────────
下一步（3 步，共约 3 分钟）：

 ① 防火墙/安全组放行上面「安全组放行」行的 TCP 端口段
 ② 把「入口地址」粘贴到 Orca 桌面端 → 自定义网络地址
 ③ 桌面机运行：tp connect <上面的配对链接>
    （输出里有给手机扫的二维码，手机 Orca 扫码即可）

有任何问题：tp doctor 会告诉你卡在哪一环。
──────────────────────────────────────────────────
NEXT
