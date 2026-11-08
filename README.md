# tunnelport

把一台有公网的 VPS 变成 **Orca 手机端的稳定入口**——自托管 TCP 隧道，单二进制，MIT 开源。

**VPS 一键部署 → Orca 桌面端粘贴地址 → 手机扫码即连。** 无需账号、无控制面、无埋点。

## 下载

- **桌面端（Windows / macOS / Linux）**：[Releases](https://github.com/haochan1996/tunnelport/releases/latest) —— 下载 `tunnelport_系统_架构`（Windows 是 `tunnelport_windows_amd64.exe`），双击即用
- **服务端（VPS）**：一条命令安装

```sh
curl -fsSL https://haochan1996.github.io/tunnelport/install.sh | sh
```

完整图文指南见 **[docs/GUIDE.md](docs/GUIDE.md)**。

## 特性

- **自研隧道引擎**：TLS 链路加密 + token 鉴权 + 证书指纹防中间人（TOFU），无杀软特征（见 [docs/AV.md](docs/AV.md)）
- **多设备**：一个配对链接，多台电脑自动分配独立入口端口
- **五段诊断**：`tp doctor` 本地 → 中继 → 鉴权 → 隧道 → 外部，逐段判定卡在哪一环
- **桌面端**：图形界面、托盘常驻、开机自启、断线自动重连、检查更新
- **端到端加密**（`--tls-terminate`）：中继只见密文
- 单二进制零依赖；systemd / launchd / Docker 开箱即用

## 目录

```
cmd/tp/               命令行：serve / connect / status / doctor / logs
cmd/tp-app/           桌面端（webview 图形界面）
internal/engine/      自研隧道引擎（rathole 为可选逃生舱）
internal/config/      状态与 token 存储（~/.tunnelport）
internal/certs/       证书生成与指纹（TOFU）
internal/diag/        五段诊断
site/                 产品站与一键安装脚本
docs/                 使用指南 / 杀软说明 / 签名计划 / 设计决策
```

## 构建

```sh
go build -o bin/tp ./cmd/tp
go build -o bin/tunnelport ./cmd/tp-app
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/tp-linux ./cmd/tp
```

## 本地冒烟（单机全链路，Windows 用 WSL）

```sh
python3 -m http.server 6768 --bind 127.0.0.1 &          # 假装 Orca
./tp serve --bind 127.0.0.1 --port-base 30100 --entry-host 127.0.0.1 &
# 输出里有配对链接 tunnelport://REDACTED
./tp connect '<配对链接>'
curl http://127.0.0.1:30101/          # 穿隧道 → 应看到 6768 的响应
curl http://127.0.0.1:30102/health    # 健康端点 → OK rtt=..ms
./tp doctor --host 127.0.0.1
```

注：单机测试时 serve 的公开端口要与 connect 的本地端口错开（真实部署在两台机器上天然分离）。

## 发布

打 tag 即发：CI 自动多平台构建（Windows / macOS / Linux × amd64 / arm64）→ 附 `SHA256SUMS` 发布到 Releases。

## 文档

| 文档 | 内容 |
|---|---|
| [docs/GUIDE.md](docs/GUIDE.md) | 安装 / 使用 / 多设备 / 排障 / rathole 逃生舱 |
| [docs/CLI.md](docs/CLI.md) | 命令行完整参考（serve / connect / status / doctor / logs） |
| [docs/AV.md](docs/AV.md) | 杀软误报说明与申诉材料 |
| [docs/SIGNING.md](docs/SIGNING.md) | 代码签名计划（SignPath） |
| [docs/DECISIONS.md](docs/DECISIONS.md) | 架构设计决策记录 |

## License

[MIT](LICENSE) · 用爱发电：[请作者喝杯咖啡](https://haochan1996.github.io/tunnelport/)
