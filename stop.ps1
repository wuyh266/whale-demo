$ErrorActionPreference = 'Stop'
& powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'stop-sovits.ps1')
$pidFile = Join-Path $PSScriptRoot 'data\demo.pid'
if (-not (Test-Path -LiteralPath $pidFile)) { Write-Host '没有找到由启动脚本运行的鲸伴服务。'; exit 0 }
$demoProcessId = [int](Get-Content -LiteralPath $pidFile -Raw).Trim()
$demoProcess = Get-Process -Id $demoProcessId -ErrorAction SilentlyContinue
if ($null -eq $demoProcess) { Write-Host '鲸伴已经停止。'; exit 0 }
$expectedPath = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot 'bin\whale-demo.exe'))
if ($demoProcess.Path -ne $expectedPath) { Write-Host '记录的进程已变化，未执行停止。'; exit 1 }
Stop-Process -Id $demoProcessId
Remove-Item -LiteralPath $pidFile
Write-Host '鲸伴已停止，聊天与记忆仍然保存在 data 文件夹。'
