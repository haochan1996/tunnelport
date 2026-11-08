Get-Process tunnelport -ErrorAction SilentlyContinue | Stop-Process -Force
Add-Type -AssemblyName System.Drawing
Add-Type @'
using System;
using System.Runtime.InteropServices;
public class W {
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int cmd);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out R rect);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr hWnd, IntPtr hdc, uint flags);
  public struct R { public int L, T, Rt, B; }
}
'@
Start-Process -FilePath 'D:\ai\orca\workspace\tp-server\bin\tunnelport.exe'
Start-Sleep -Seconds 9
$p = Get-Process tunnelport -ErrorAction SilentlyContinue | Where-Object { $_.MainWindowHandle -ne 0 } | Select-Object -First 1
if (-not $p) { Write-Host 'NO-WINDOW'; exit }
[W]::ShowWindow($p.MainWindowHandle, 3) | Out-Null   # SW_MAXIMIZE
[W]::SetForegroundWindow($p.MainWindowHandle) | Out-Null
Start-Sleep -Seconds 2
$r = New-Object W+R
[W]::GetWindowRect($p.MainWindowHandle, [ref]$r) | Out-Null
$w = $r.Rt - $r.L; $h = $r.B - $r.T
Write-Host "window: ${w}x${h}"
$bmp = New-Object System.Drawing.Bitmap $w, $h
$g = [System.Drawing.Graphics]::FromImage($bmp)
$hdc = $g.GetHdc()
[W]::PrintWindow($p.MainWindowHandle, $hdc, 2) | Out-Null   # PW_RENDERFULLCONTENT
$g.ReleaseHdc($hdc)
$bmp.Save('D:\ai\orca\workspace\tp-server\bin\relink6.png')
Get-Process tunnelport -ErrorAction SilentlyContinue | Stop-Process -Force
Write-Host ok
