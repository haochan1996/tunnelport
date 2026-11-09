# tunnelport Windows 一键安装
#
#   serve（VPS）：   powershell -c "irm https://haochan1996.github.io/tunnelport/install.ps1 | iex"
#   connect（电脑）：powershell -c "$env:TP_ROLE='connect'; $env:TP_URL='<配对链接>'; irm https://haochan1996.github.io/tunnelport/install.ps1 | iex"
#
# 自动完成：下载 → 安装到用户目录（无需管理员）→ 注册开机自启（计划任务）→ 打印地址。
# 环境变量：TP_VERSION（默认 v1.0.0）、TP_URL（connect 模式的配对链接）
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
# 提示：想装桌面图形界面的用户请直接下载 tunnelport_windows_amd64.exe（GitHub Release），
# 本脚本安装的是命令行工具 tp（服务器/无人值守场景）。
$ErrorActionPreference = "Stop"

$version = if ($env:TP_VERSION) { $env:TP_VERSION } else { "v1.0.3" }
$role = if ($env:TP_ROLE) { $env:TP_ROLE } else { "serve" }
$url = $env:TP_URL
if ($role -eq "connect" -and -not $url) {
    Write-Host "✗ 连接模式需要配对链接：  `$env:TP_URL='tunnelport://…' 后重跑"
    exit 1
}

$base = "https://github.com/haochan1996/tunnelport/releases/download/$version"
$dl = "$base/tp_windows_amd64.exe"
$dir = Join-Path $env:LOCALAPPDATA "tunnelport"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$dest = Join-Path $dir "tp.exe"

Write-Host "==> [1/3] 下载 tp ($version)"
try {
    Invoke-WebRequest -Uri $dl -OutFile $dest -TimeoutSec 120 | Out-Null
} catch {
    Write-Host "    主源较慢，切换国内镜像…"
    try {
        Invoke-WebRequest -Uri "https://gh-proxy.com/$dl" -OutFile $dest -TimeoutSec 180 | Out-Null
    } catch {
        Write-Host "✗ 下载失败（主源+镜像均不通）。请手动下载：$dl"
        exit 1
    }
}
Write-Host "==> [2/3] 安装到 $dir"

Write-Host "==> [3/3] 注册开机自启（计划任务，崩溃自动重启）"
$argList = $role
if ($url) { $argList = "$role `"$url`"" }
$action = New-ScheduledTaskAction -Execute $dest -Argument $argList
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
    -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -ExecutionTimeLimit ([TimeSpan]::Zero)
try {
    Register-ScheduledTask -TaskName "tunnelport" -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
    Start-ScheduledTask -TaskName "tunnelport" -ErrorAction SilentlyContinue
} catch {
    Write-Host "⚠ 注册计划任务失败（$($_.Exception.Message)）；可直接运行 $dest $argList"
}

Start-Sleep -Seconds 5
Write-Host ""
Write-Host "════════════════════════════════════════════"
if ($role -eq "serve") {
    Write-Host " 部署完成！地址 / 配对链接 / 二维码："
    Write-Host "════════════════════════════════════════════"
    & $dest status --show-pairing
} else {
    Write-Host " 连接状态："
    Write-Host "════════════════════════════════════════════"
    & $dest status
}
Write-Host ""
Write-Host "下一步：serve 模式在防火墙放行提示的端口段；入口地址粘贴到 Orca；手机扫 Orca 生成的二维码。"
