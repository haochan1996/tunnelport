# tunnelport 使用指南

> 三个角色，三步连通：**VPS（入口）→ 电脑（Orca + tp connect）→ 手机（Orca 扫码）**。

```
手机 Orca ──→ VPS 入口 ──→ 你的电脑 ──→ Orca 6768
            （第1步装）   （第2步装）
```

---

## 第 1 步 · VPS 上装入口（一条命令）

在有公网 IP 的服务器（推荐国内轻量服务器）上运行：

```sh
curl -fsSL https://haochan1996.github.io/tunnelport/install.sh | sh
```

**自动完成**：下载二进制、注册开机自启、打印地址与配对链接。**不需要手动下载任何文件。**

输出里记住两样东西：

| 输出 | 用途 |
|---|---|
| **入口地址**（如 `1.2.3.4:30001`） | 第 3 步粘贴到 Orca |
| **配对链接**（`tunnelport://…`） | 第 2 步在电脑上使用 |

然后在云控制台**放行输出里提示的 TCP 端口段**（默认 30000-30002）。

> 服务端一键脚本（进阶）：Linux / macOS `curl | sh`、Windows `irm | iex`；Docker 见文末。日常用户直接下载软件即可。

---

## 第 2 步 · 电脑装桌面软件（下载即用）

从 [Releases](https://github.com/haochan1996/tunnelport/releases) 下载 **`tunnelport_`** 开头的桌面软件
（Windows / macOS / Linux 都有），然后：

1. **双击打开**
2. 在输入框**粘贴第 1 步的配对链接** → 点「连接」
3. 看到「已连接」即成功——软件保持连接、自动重连；勾选「开机自动连接」后重启也无感

没有命令行、没有配置文件。配对码丢了不用慌：`tp status --show-pairing` 随时取回。

> 有 NAS 的用户可让 NAS 替电脑连接（电脑零安装）：`tp connect <配对链接> --local-host <电脑的局域网IP>`（进阶）。


## 多台电脑同时连（多设备）

**两台电脑用同一个配对链接即可**——服务端自动为每台分配独立入口：

```sh
# 电脑 A 和电脑 B 都运行同一条命令
tp connect <同一个配对链接>
# 电脑 A → 自动获得入口 VPS:30301
# 电脑 B → 自动获得入口 VPS:30303
```

然后各台电脑的 Orca **粘贴各自的入口地址**，手机分别扫码配对（Orca 支持多设备切换）。
每台电脑占 2 个端口（入口 + 健康自检），防火墙放行范围相应放宽：`--devices N`
时放行 `base 到 base+2N` 一段。serve 端 `--devices N` 可限设备数（默认不限）。

## 第 3 步 · 手机连上（Orca 自带配对）

1. 电脑端 Orca「自定义网络地址」里粘贴**入口地址**（`1.2.3.4:30001`）
2. Orca 打开 **Set up Mobile** → 桌面显示配对二维码
3. 手机 Orca 扫码 → 连通

> 不要填 `wss://` 开头的地址（Orca 桌面端已知不兼容）。

**验证**（可选）：手机关 Wi-Fi 用 4G，浏览器开 `http://入口地址后段端口+1/health`（如 `1.2.3.4:30002/health`），显示 `OK` 即全链路通畅。

---

## 连不上？

在电脑上运行 `tp doctor`——五段诊断会直接告诉你卡在哪一环、怎么处理。

常见情况：

- **双击 tp.exe 弹「This is a command line tool…」**：下载的是命令行版。
  桌面图形界面版叫 **`tunnelport_windows_amd64.exe`**，重新下载它双击即用
- **Windows 弹 SmartScreen**（"Windows 已保护你的电脑"）：未签名软件的通用提示，点「更多信息」→「仍要运行」即可
- **杀软删除**（罕见）：自研引擎无特征一般不会被拦；真遇到看 [杀软误杀手册](AV.md)（申诉材料已备）
- **走代理的网络**：为服务器 IP 添加 DIRECT 直连规则
- **服务器 IP 变了**：家宽需 DDNS；用 VPS 则不会变

---

## 高级 · rathole 逃生舱（应急用，平时不需要）

tunnelport 默认用**自研引擎**——无杀软特征、单二进制，日常场景足够。
**rathole 逃生舱**是备用引擎，只在极端排查时用（例如怀疑自研引擎本身有 bug，
需要换一个引擎做对照排除）。**普通用户不需要、也不建议使用**：rathole 二进制带
`HackTool/Rathole.a` 专杀特征，腾讯管家等杀软会直接删除（见 [杀软误杀手册](AV.md)）。

### 什么时候才用

- `tp doctor` 显示中继 / 隧道段异常，且网络和服务器都确认没问题，需要对照排除引擎因素
- 与还在跑 rathole 引擎的旧版本做互通排查

### 使用前必读

1. **两端必须同时切换**：自研引擎与 rathole 协议不通，serve 和 connect 不能混用
2. **需要单独下载 rathole 二进制**（v0.5.0 固定版本）——tunnelport 不内嵌它：
   内嵌会被杀软的专杀特征连带标记整个程序
3. **配对链接不变**：切换引擎不换 token，不需要重新配对

### 第 1 步 · 准备 rathole 二进制

从 [rathole 官方发布页](https://github.com/rathole-org/rathole/releases/tag/v0.5.0) 下载 v0.5.0：

| 平台 | 文件 |
|---|---|
| Linux x86_64 | `rathole-x86_64-unknown-linux-gnu.zip` |
| Linux ARM64 | `rathole-aarch64-unknown-linux-musl.zip`（静态编译，各发行版通用） |
| macOS（Apple 芯片） | 官方 v0.5.0 未提供原生构建：用 Intel 版（下面一行，Rosetta 2 转译运行）或自行编译 |
| macOS（Intel） | `rathole-x86_64-apple-darwin.zip` |
| Windows | `rathole-x86_64-pc-windows-msvc.zip` —— 会被杀软删除，**建议在 WSL 里运行** |

解压得到 `rathole`（Windows 是 `rathole.exe`），放到任意目录，记下完整路径。
仓库里的 `scripts/fetch-rathole.sh` 可一键下载（Linux / macOS）。

### 第 2 步 · 两端都切换

```sh
# VPS（serve）：在原有参数上追加 --engine rathole --rathole <rathole 路径>
tp serve --port-base 30000 --engine rathole --rathole /usr/local/bin/rathole

# 电脑（connect）：同样追加两个参数
tp connect <配对链接> --engine rathole --rathole /home/you/tools/rathole
```

- **用 systemd 管理 serve 的**：把这两个参数加进 unit 的 `ExecStart`，
  然后 `sudo systemctl daemon-reload && sudo systemctl restart tunnelport`
- **用桌面 GUI 的**：GUI 暂不支持切换引擎（设置里的 rathole 选项为灰）。
  要走逃生舱就用命令行 `tp connect`，并先把 GUI 的隧道「暂停」或退出 GUI，
  避免本地端口被 GUI 的连接占用
- **找不到 rathole 时**程序会报错提示 `--rathole` 指定路径，不会静默失败

### 切回自研（默认）

去掉 `--engine rathole --rathole …` 这两个参数重启即可，同样不用重新配对。
rathole 只是应急对照用的"逃生舱"，用完就切回来。

---

## 其他安装方式

> 命令行完整参数见 [CLI 参考](CLI.md)。

<details>
<summary>Docker（NAS / 容器环境）</summary>

```sh
docker run -d --restart unless-stopped \
  -e TP_ROLE=serve -e TP_PORT_BASE=30123 -e TP_TOKEN=<token，可留空自动生成> \
  -p 30123-30125:30123-30125 \
  -v tp-state:/root/.tunnelport \
  ghcr.io/haochan1996/tunnelport:v0.1.0
```

客户端：`TP_ROLE=connect` + `TP_TOKEN=<配对链接>`，无需端口映射。

</details>

<details>
<summary>二进制手动安装（VPS 不用 install.sh 时）</summary>

```sh
curl -fL https://github.com/haochan1996/tunnelport/releases/download/v0.1.0/tp_linux_amd64 -o tp
chmod +x tp && sudo mv tp /usr/local/bin/
sudo systemd-run -u tunnelport -- /usr/local/bin/tp serve   # 或自己写 unit
```

macOS / Windows：Releases 直接下载，命令相同（`tp serve` / `tp connect`）。

</details>

<details>
<summary>开机自启（电脑端）</summary>

- Linux：systemd user unit（`loginctl enable-linger`）
- macOS：launchd；Windows：计划任务（桌面端设置里一键开关）

</details>

<details>
<summary>常用命令</summary>

| 命令 | 作用 |
|---|---|
| `tp status` | 当前地址与状态 |
| `tp doctor` | 五段诊断 |
| `tp logs -f` | 引擎日志 |
| `tp serve --regenerate` | 重置 token（所有客户端需重新配对） |

</details>

<details>
<summary>端口说明</summary>

默认使用连续三端口（控制 / Orca 服务 / 健康自检各一），防火墙放行一段即可。
`--port-base N` 可指定基座；被占用时会报错提示，不会擅自换端口。

</details>

<details>
<summary>安全与隐私</summary>

- 隧道链路 TLS 加密，配对码携带证书指纹防中间人
- 端到端模式（`tp connect --tls-terminate`）：中继只见密文
- 代码开源（MIT），不运营任何节点、不记录任何内容
- token 泄露时用 `tp serve --regenerate` 重置

</details>
