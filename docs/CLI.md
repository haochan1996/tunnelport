# tp 命令行参考

服务端与命令行用户的手册。桌面用户直接用图形界面即可，无需阅读本文。

所有命令均为单二进制 `tp`（Windows 为 `tp.exe`），子命令如下：

| 命令 | 用途 | 典型使用方 |
|---|---|---|
| `tp serve` | 在有公网的机器上起中继 | VPS |
| `tp connect` | 连接中继，映射本地 Orca 端口 | 电脑 / NAS |
| `tp status` | 查看状态、取回配对链接 | 任意 |
| `tp doctor` | 五段诊断，定位卡在哪一环 | 任意 |
| `tp logs` | 查看引擎日志 | 任意 |

---

## tp serve（服务端）

在有公网 IP 的机器上运行，输出入口地址与配对链接。

```sh
tp serve --port-base 30000 --entry-host <你的公网IP或域名>
```

### 参数

| 参数 | 环境变量 | 默认 | 说明 |
|---|---|---|---|
| `--port-base N` | `TP_PORT_BASE` | 自动选择 | 三连端口基座：控制=N、入口=N+1、健康=N+2。防火墙放行一段即可。显式指定被占用会报错而不是擅自换 |
| `--port N` | `TP_PORT` | 自动 | 控制通道端口（与 `--port-base` 二选一） |
| `--bind ADDR` | — | `0.0.0.0` | 控制通道监听地址 |
| `--public-port N` | `TP_PUBLIC_PORT` | `6768` | 入口端口（手机连这个）。被占用自动顺延并提示 |
| `--health-port N` | `TP_HEALTH_PORT` | `6769` | 健康端点端口（高级覆盖，与 `--port-base` 互斥） |
| `--devices N` | `TP_DEVICES` | `0`（不限） | 最大设备数。每台电脑占 2 个端口（入口+健康），先到先得 |
| `--entry-host HOST` | `TP_ENTRY_HOST` | 自动检测 | 显式指定入口主机名/IP。代理网络、软路由、DDNS 域名必填 |
| `--token TOKEN` | `TP_TOKEN` | 自动生成 | 指定 token（52 位大写字母和数字） |
| `--regenerate` | — | 关 | 重新生成 token；**所有已配对客户端需重新配对** |
| `--engine native\|rathole` | — | `native` | 隧道引擎。rathole 为逃生舱，见 [GUIDE](GUIDE.md) |
| `--rathole PATH` | — | PATH 查找 | rathole 可执行文件路径（`--engine rathole` 时用） |

### 输出

启动后输出三样东西：

1. **入口地址** —— 粘贴到 Orca 桌面端「自定义网络地址」
2. **配对链接**（`tunnelport://…`）—— 电脑端 `tp connect` 用；随时可用 `tp status --show-pairing` 取回
3. **安全组放行提示** —— 云服务器需在控制台放行对应端口段

### token 与配置

- token 首次自动生成并**持久化**（`TP_CONFIG_DIR` 或 `~/.tunnelport/`），重启复用，配对链接长期有效
- 重置 token：`tp serve --regenerate`（所有客户端需重新配对）
- 用 systemd 跑服务时建议 `Environment=TP_CONFIG_DIR=/etc/tunnelport`，任何用户都能 `tp status --show-pairing` 取回链接

### 多设备

同一个配对链接可被多台电脑同时使用；每连入一台自动分配独立端口槽（入口+健康各一）：

```
设备 1 → <IP>:N+1 / <IP>:N+2
设备 2 → <IP>:N+3 / <IP>:N+4
…
```

`--devices N` 限上限。防火墙放行范围相应放宽为 `N 到 N+2*设备数`。

### systemd 示例

```ini
[Unit]
Description=tunnelport
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=120
StartLimitBurst=5

[Service]
Environment=TP_CONFIG_DIR=/etc/tunnelport
ExecStart=/usr/local/bin/tp serve --port-base 30000 --entry-host 你的公网IP
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

---

## tp connect（电脑端）

```sh
tp connect '<配对链接>'
```

### 参数

| 参数 | 环境变量 | 默认 | 说明 |
|---|---|---|---|
| 第 1 个参数 | `TP_TOKEN` | — | 配对链接 `tunnelport://host:控制端口?public=入口端口&fp=证书指纹#TOKEN`，或纯 token（配 `--host/--port`） |
| `--host HOST` | — | — | 中继地址（纯 token 时必填） |
| `--port N` | `TP_CONTROL_PORT` | — | 中继控制端口（URL 自带；纯 token 时必填） |
| `--local-port N` | — | `6768` | 本地 Orca 端口 |
| `--local-host ADDR` | — | `127.0.0.1` | Orca 所在地址。NAS 场景填电脑的局域网 IP（如 `192.168.1.10`），电脑零安装 |
| `--tls-terminate` | — | 关 | 端到端加密：本地终止 TLS，中继只见密文；手机端需接受 `https://` |
| `--transport tls\|tcp` | — | `tls` | 传输层。`tcp` 用于对端旧版本回退 |
| `--engine native\|rathole` | — | `native` | 隧道引擎（须与 serve 一致） |
| `--rathole PATH` | — | PATH 查找 | rathole 路径 |

断线自动重连（1s→30s 退避）。`Ctrl-C` 退出，端口即释放。

---

## tp status

```sh
tp status                 # 角色 / 入口地址 / 控制通道 / token 指纹 / 版本
tp status --show-pairing  # 追加完整配对链接（含 token，勿外泄）
tp status --show-pairing --qr   # 再追加二维码
```

- `token 未保存` 提示：token 只在内存/服务侧，用 `tp serve` 重启一次落盘，或 `--regenerate` 重置
- 多设备场景下显示的是当前分配到的端口

## tp doctor

```sh
tp doctor [--host HOST] [--port N] [--local-port N]
```

五段逐项判定，告诉你卡在哪一环：

1. **本地** —— 本地 Orca 端口是否监听
2. **中继** —— 服务器是否可达、TLS 握手
3. **鉴权** —— token 是否被接受、证书指纹是否与配对码一致
4. **隧道** —— 会话是否建立、上下行转发
5. **外部** —— 健康端点（需手机 4G 等外部网络验证）

## tp logs

```sh
tp logs [-n 50] [-f]
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `-n, --lines N` | 50 | 显示末尾行数 |
| `-f, --follow` | 关 | 持续跟踪（类似 `tail -f`） |

日志仅存本机（`~/.tunnelport/engine.log`），不上传。

## 环境变量对照

| 变量 | 等价参数 | 作用 |
|---|---|---|
| `TP_CONFIG_DIR` | — | 配置/状态目录（默认 `~/.tunnelport`） |
| `TP_TOKEN` | `--token` | token 或配对链接 |
| `TP_PORT` / `TP_PORT_BASE` | `--port` / `--port-base` | 控制端口 / 端口基座 |
| `TP_PUBLIC_PORT` / `TP_HEALTH_PORT` | `--public-port` / `--health-port` | 入口 / 健康端口 |
| `TP_DEVICES` | `--devices` | 最大设备数 |
| `TP_ENTRY_HOST` | `--entry-host` | 入口主机覆盖 |
| `TP_CONTROL_PORT` | `--port`（connect） | 控制端口 |

## 常见报错

| 报错 | 含义与处理 |
|---|---|
| `配对码不完整：缺…` | 配对链接截断了。整条复制 `tunnelport://…#TOKEN`，或服务端 `tp status --show-pairing` 取回 |
| `token 不匹配，重新复制` | token 应为 52 位大写字母和数字；重新复制配对链接 |
| `端口 N 绑定失败` | 显式指定的端口被占用。换端口或释放占用（`--port-base` 显式指定不会自动换） |
| `PATH 中未找到 rathole` | `--engine rathole` 需要单独下载 rathole，用 `--rathole` 指定路径（见 [GUIDE](GUIDE.md) 逃生舱一节） |
| `证书获取失败：TLS 探测失败` | 网络不通或对端旧版本。检查防火墙；旧版本可 `--transport tcp` 回退 |
| `设备数已满` | 达到 `--devices` 上限，调大或释放空闲连接 |
