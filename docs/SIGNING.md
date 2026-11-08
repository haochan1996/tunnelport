# Windows 代码签名（SignPath Foundation · 开源免费）

> 目标：消灭 SmartScreen「已保护你的电脑」拦截与杀软误删。私钥在 SignPath 的
> 硬件安全模块（HSM）里，仓库不用存任何证书文件。CI 配置已就绪，
> 申请通过后填 4 个 Secrets 即自动生效。

## 申请（一次性，1-3 个工作日邮件审核）

**入口**：https://signpath.org/apply.html

**资格自查**（我们全满足）：OSI 许可证（MIT ✅）· 公开仓库 ✅ · 免费分发 ✅ ·
无恶意代码 ✅ · 有既往 release（v0.1.0 ✅）· GitHub/SignPath 账号开 MFA

**申请表内容（直接粘贴）**：

- Repository URL: `https://github.com/haochan1996/tunnelport`
- Project description: `tunnelport — self-hosted TCP tunnel for Orca remote development.
  Open source (MIT), CLI + desktop GUI, builds released on GitHub Releases.`
- Why you need code signing: `Unsigned Windows binaries trigger SmartScreen warnings
  ("Windows protected your PC") and are deleted by antivirus heuristics.
  Signing will protect our users from these false positives.`
- Contact email: 你的常用邮箱（审核邮件往来用）
- Maintainer identity: `haochan1996 (GitHub) — repository owner`

## 通过后配置（10 分钟）

1. **SignPath 控制台**（signpath.io，GitHub 登录）：
   - Organization → 添加 Trusted Build System：**GitHub.com**
   - **Projects** → 添加 `tunnelport`，Repository URL 填本仓库
   - **Signing Policies** → 新建：Slug 填 `public-release`，类型 Authenticode，目标 `.exe`
2. **安装 SignPath GitHub App**：https://github.com/apps/signpath → 选择本仓库授权
   （作用：允许 SignPath 下载 CI 产物并回传签名结果）
3. **仓库 Secrets**（Settings → Secrets and variables → Actions）添加 4 个：

| Secret | 值来源 |
|---|---|
| `SIGNPATH_API_TOKEN` | SignPath 控制台 → Settings → API Tokens |
| `SIGNPATH_ORGANIZATION_ID` | SignPath 控制台 → Organization 页的 ID（UUID） |
| `SIGNPATH_PROJECT_SLUG` | `tunnelport`（或你在控制台填的 slug） |
| `SIGNPATH_SIGNING_POLICY_SLUG` | `public-release` |

## 生效方式

`.github/workflows/release.yml` 的 `sign` job **已接好**：只要 `SIGNPATH_API_TOKEN`
存在就自动签名 Windows 产物，并在发布时**优先使用签名版**（`Prefer signed Windows
binaries` 步骤）。Secrets 未配置时流水线照常发布未签名版，互不影响。

签名标志验证（下载后）：

```powershell
Get-AuthenticodeSignature .\tunnelport_windows_amd64.exe
# Status: Valid 且 SignerCertificate 是 SignPath Foundation 即成功
```
