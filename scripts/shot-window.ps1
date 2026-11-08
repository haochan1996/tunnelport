Get-Process tunnelport -ErrorAction SilentlyContinue | Stop-Process -Force
Add-Type @'
using System;
using System.Runtime.InteropServices;
public class W {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out R rect);
  public struct R { public int L, T, Rt, B; }
}
'@
[W]::SetProcessDPIAware() | Out-Null
$env:TP_TAB = '3'
Start-Process -FilePath 'D:\ai\orca\workspace\tp-server\bin\tunnelport.exe' -ArgumentList @('tunnelport://REDACTED '--local-host', '127.0.0.1')
Start-Sleep -Seconds 8
Add-Type -AssemblyName System.Drawing
$p = Get-Process tunnelport -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
if (-not $p) { Write-Host 'NO-WINDOW'; exit }
[W]::SetForegroundWindow($p.MainWindowHandle) | Out-Null
Start-Sleep -Seconds 2
$r = New-Object W+R
[W]::GetWindowRect($p.MainWindowHandle, [ref]$r) | Out-Null
$w = $r.Rt - $r.L; $h = $r.B - $r.T
Write-Host "physical window: ${w}x${h} at $($r.L),$($r.T)"
$bmp = New-Object System.Drawing.Bitmap $w, $h
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($r.L, $r.T, 0, 0, $bmp.Size)
$bmp.Save('D:\ai\orca\workspace\tp-server\bin\relink4.png')
Get-Process tunnelport -ErrorAction SilentlyContinue | Stop-Process -Force
Write-Host ok
