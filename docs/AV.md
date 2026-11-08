# 杀毒软件误杀：现状、防线与申诉手册

> 未签名的网络工具类软件被杀软拦截是行业通病（frp、rathole 等同类工具全中，
> rathole 甚至有专杀特征 `HackTool/Rathole.a`）。tunnelport 从架构上规避了大部分
> 风险（自研引擎、无第三方特征、不落盘额外二进制），剩余风险靠下面三层防线闭环。

## 现状评估（2026-11-08 实测）

| 项 | 状态 |
|---|---|
| 自研引擎（无第三方特征） | ✅ 本地实测运行正常，腾讯管家未拦截（tp / tunnelport 桌面端均已跑通） |
| rathole（`--engine rathole` 逃生舱） | ❌ 被腾讯管家专杀删除——**普通用户不要用**，仅应急 |
| 未签名软件的云端信誉 | ⚠️ 随下载量变化；未签名 + 联网 + 开机自启 属于启发式关注对象 |

## 第一层：代码签名（治本，免费路径）

> **详细申请手册见 [SIGNING.md](SIGNING.md)**（含可直接粘贴的申请文案 + 4 个 Secrets 配置表）。

**SignPath 为开源项目提供免费代码签名**（EV 级信任，签完 Windows SmartScreen /
多数杀软直接放行）：

1. 到 [signpath.io](https://signpath.io) 用 GitHub 账号登录 → 创建 Organization
2. 「Projects」→ 添加本仓库（`haochan1996/tunnelport`）→ 建 **Public Release** 签名策略
3. 安装 **SignPath GitHub App**（需要仓库管理员在浏览器里授权）
4. 仓库 Settings → Secrets 添加 `SIGNPATH_API_TOKEN`（SignPath 控制台生成）
5. CI 加签名步骤：

```yaml
  sign:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with: { path: dist, merge-multiple: true }
      - uses: signpath/github-action-submit-signing-request@v1
        with:
          api-token: ${{ secrets.SIGNPATH_API_TOKEN }}
          organization-id: '<SignPath 控制台里的 Organization ID>'
          project-slug: 'tunnelport'
          signing-policy-slug: 'public-release'
          github-artifact-id: '${{ steps.upload.outputs.artifact-id }}'
          wait-for-completion: true
          output-artifact-directory: signed/
```

签名后的产物再进 Release。**签名落地前**，下面两层兜底。

## 第二层：杀软误报申诉（发布后做，一次签名长期受益）

向各厂商提交误报申诉时带上：软件名、官网/仓库地址、SHA256、用途说明。
**申诉话术模板**：

> 这是开源软件 tunnelport（MIT，仓库 github.com/haochan1996/tunnelport），
> 用于 Orca 远程开发的自托管 TCP 隧道，源码可审计，无任何恶意行为。
> 被误报的文件 SHA256：`<哈希>`。请加入白名单。

| 厂商 | 申诉入口 | 备注 |
|---|---|---|
| Microsoft Defender | https://www.microsoft.com/wdsi/filesubmission | 提交后 72h 内评估，通了全球生效 |
| 腾讯电脑管家 | 腾讯电脑管家客户端内「反馈/误报申诉」，或 txc.qq.com 提工单 | 通常 3-7 天 |
| 360 安全卫士 | 搜索「360 软件误报申诉」→ 360 官方工单入口 | 通常 3-7 天 |
| 火绒 | 火绒客户端内「误报反馈」，或 bbs.huorong.cn 误报版块 | 通常 1-2 周 |
| 首检 | https://www.virustotal.com 上传样本看各家判定 | 决定要申诉哪几家 |

## 第三层：用户侧白名单（即时可用）

- tp 检测到引擎/自身被删时会报 **TP-6001**，指引到杀软「信任区」添加目录
- Windows 推荐路径：杀软设置 → 信任区/排除项 → 添加 `安装目录`
- 命令行用户：WSL 内运行（多数杀软不扫 WSL 文件系统，实测有效）

## 当前发布哈希（v0.1.0，供申诉引用）

```
c8d7b6a7a4d86d926c060c1a2a337247cd83a3404164403e0076e5101fac220b  tp_windows_amd64.exe
1f5a8ff31c694231fa00bd5c7f9ad740e5d1b9646dab0cbd004dfde6b3c6e87b  tunnelport_windows_amd64.exe
```

（每个 Release 附 `SHA256SUMS`，以最新为准。）
