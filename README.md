# tunnelport 内部仓库（私有）

源码 + PRD + 发布流水线。**不公开**。公开仓是 [haochan1996/tunnelport](https://github.com/haochan1996/tunnelport)（站点 + Release 二进制）。

## 目录

```
cmd/tp/               CLI 入口：serve / connect / status / doctor / service
internal/config/      状态存储（~/.tunnelport，token 只存指纹）
internal/tokenx/      token 生成 / 配对 URL 解析
internal/detect/      出口 IP、CGNAT、端口探测（FR-1）
internal/engine/      Engine 接口 + rathole 实现（临时配置 + 保活重启）
internal/address/     入口地址 / 二维码
internal/diag/        tp doctor 五段诊断
.github/workflows/    release.yml：打 tag → 多平台构建 → 推 Release 到公开仓
scripts/              开发辅助脚本
```

## 构建

```sh
go build -o bin/tp ./cmd/tp
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/tp-linux ./cmd/tp
```

rathole 从上游下载（开发用）：`./scripts/fetch-rathole.sh`

## 本地冒烟（单机全链路，Windows 用 WSL）

```sh
# WSL 内
python3 -m http.server 6768 --bind 127.0.0.1 &            # 假装 Orca
./tp serve --bind 127.0.0.1 --port 30123 --public-port 17668 --rathole ./rathole &
TOKEN=$(grep -oE '[A-Z2-7]{52}' /tmp/serve.log | head -1)  # serve 输出里抄
./tp connect --host 127.0.0.1 --port 30123 --token $TOKEN --rathole ./rathole &
curl http://127.0.0.1:17668/        # 穿隧道 → 应看到 6768 的响应
curl http://127.0.0.1:6769/health   # 健康端点 → OK rtt=..ms
./tp doctor --host 127.0.0.1 --port 30123
```

注意：**单机测试必须 `--public-port` 错开本地 6768**（serve 侧公开端口和 connect 侧本地端口在同一台机器上会冲突；真实部署天然分离）。rathole v0.5.0 的配置 schema 与主分支文档不同，配置生成已按 tag 源码对齐（PRD D-1 教训）。

## 发布

1. `git tag v0.x.0 && git push --tags`
2. CI 自动构建 5 个平台 → `SHA256SUMS` → 以 draft release 推到公开仓
3. 前置配置（一次性）：创建对公开仓有 `contents:write` 权限的 PAT，
   加到本仓 secret `RELEASE_TOKEN`

## 已知问题 / 下一步

- 传输层 tcp+token，TLS（D-8）W1.5
- doctor C/D 段需要活跃会话数据（W2 从引擎取）
- 自启 `tp service`（W2）
- 杀软误报（腾讯管家实测删 rathole）：已做 TP-6001 检测，提白名单流程待办
