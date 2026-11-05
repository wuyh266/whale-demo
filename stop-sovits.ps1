$ErrorActionPreference = 'Stop'
try {
    $processFile = Join-Path $PSScriptRoot 'data\sovits-process.json'
    if (-not (Test-Path -LiteralPath $processFile)) { Write-Host '没有由鲸伴脚本启动的本地语音进程。'; exit 0 }
    $record = Get-Content -LiteralPath $processFile -Raw -Encoding UTF8 | ConvertFrom-Json
    $processInfo = Get-Process -Id ([int]$record.pid) -ErrorAction SilentlyContinue
    if ($null -eq $processInfo) { Remove-Item -LiteralPath $processFile; Write-Host '本地语音服务已停止。'; exit 0 }
    $expectedBridge = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot 'tools\sovits_server.py'))
    if ($processInfo.Path -ne $record.python -or $record.bridge -ne $expectedBridge -or $record.startedAtUtcTicks -ne $processInfo.StartTime.ToUniversalTime().Ticks.ToString()) {
        throw '记录的语音进程已变化，未执行停止。'
    }
    Stop-Process -Id ([int]$record.pid)
    Remove-Item -LiteralPath $processFile
    Write-Host '本地语音服务已停止；模型、配置与聊天记录均保留。'
} catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}
