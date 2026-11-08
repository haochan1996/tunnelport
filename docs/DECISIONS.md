# 技术决策记录（公开版）

商业敏感决策（定价、指标、风险应对）在内部文档中，不公开。本页只记录技术决策。

## ADR-1 隧道引擎：rathole，固定 tag v0.5.0 自构建

- 选 rathole（Rust，14k+ star）作为隧道引擎，`Engine` 接口保留替换能力
- **必须固定 tag 并以 tag 源码为准**：v0.5.0 的配置 schema（services 嵌套在
  `[server]/[client]` 内、server service 用 `bind_addr`）与主分支文档不同；
  照文档写配置会静默失败（实测踩坑）
- 上游 release 构建曾损坏（PR #436），因此不依赖上游产物，CI 自建

## ADR-2 token 存储：加密落盘，不存哈希

- rathole 的 noise/TLS 传输需要 token 原值参与握手，哈希无法完成鉴权
- 存储：macOS Keychain / Windows DPAPI / Linux 0600 + AES-GCM
- 状态目录里另存 SHA-256 指纹仅用于人眼比对，不参与任何鉴权

## ADR-3 外部可达性：中继 `/health` 端点 + 手机 4G 验证

- 无后端架构下没有外部探针；doctor 在桌面端探测入口等于重复中继检查
- 方案：中继额外暴露一条健康映射 `entry:6769 → 客户端本地 /health`，
  手机用 4G 打开即验证"隧道 + 外部可达"两段，这是无后端下唯一可信的外部视角

## ADR-4 链路加密与端到端模式

- 隧道传输默认 rathole `transport=tls`（自签证书自动生成，证书指纹随配对码
  携带，客户端 TOFU 比对，不匹配即中止并提示中间人）
- 端到端模式（`--tls-terminate`）：本地终止 TLS，手机 `https://入口:6768`
  的流量穿透隧道直达桌面解密 —— 中继只见密文
- 实现注意：PKCS#12 必须用现代算法编码（AES-256/PBES2）；遗留 RC2 会被
  OpenSSL 3 拒绝（实测踩坑）
- 上限：手机端由 Orca 应用决定（当前为明文 HTTP 客户端），端到端模式依赖
  手机端接受 `https://` 与自签证书

## ADR-5 入口地址：显式指定优先于自动检测

- 代理/软路由网络里，出口 IP 自动检测返回的是代理出口地址（实测为境外 VPS），
  做入口从外面连不进来
- `serve --entry-host` / `connect --host` 均可覆盖配对码内嵌地址；DDNS 域名走同一参数

## ADR-6 退出清理：无孤儿进程

- SIGTERM（docker stop / systemctl stop）与 Ctrl-C 同等处理：
  tp 与 rathole 子进程同步退出，端口即释（专项实测）
- Linux 对 rathole 子进程设置 `PDEATHSIG`；注意 Go 的 Pdeathsig 绑定在创建
  子进程的 OS 线程上，必须 `runtime.LockOSThread()`，否则线程回收会误杀健康子进程
  （golang/go#27505，实测踩坑）

## ADR-7 零后端架构

- 无账号、无控制面、无数据库、无埋点上报；配对靠 token，指标靠本地统计 + 访谈
- 隐私承诺：不记录、不落盘任何终端内容；中继由用户自己运行
- 商业模式为"软件免费 + 可选付费远程支持服务"，与零后端架构自洽

## ADR-8 端口策略：默认固定约定 + 冲突自动顺延

- 控制端口默认 30000–40000 自动选择（黑名单见 `internal/detect`），
  服务端口默认 6768、健康端点 6769
- 冲突时自动顺延（`NextFreePort`，同批分配互斥避让）并醒目提示手机应填的
  新地址；配对码自动携带最终端口
