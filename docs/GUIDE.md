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
| **配对链接**（`tunnelport://REDACTED | 第 2 步在电脑上使用 |

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

- **Windows 弹 SmartScreen**（"Windows 已保护你的电脑"）：未签名软件的通用提示，点「更多信息」→「仍要运行」即可
- **杀软删除**（罕见）：自研引擎无特征一般不会被拦；真遇到看 [杀软误杀手册](AV.md)（申诉材料已备）
- **走代理的网络**：为服务器 IP 添加 DIRECT 直连规则
- **服务器 IP 变了**：家宽需 DDNS；用 VPS 则不会变

---

## 其他安装方式

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
- macOS：launchd；Windows：计划任务（W2 提供 `tp service install`）

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
