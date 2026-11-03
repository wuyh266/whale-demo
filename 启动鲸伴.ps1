param([switch]$NoBrowser)
$ErrorActionPreference = 'Stop'
try {
    Set-Location -LiteralPath $PSScriptRoot
    $demoAddr = '127.0.0.1:8787'
    if ($env:APP_ADDR) { $demoAddr = $env:APP_ADDR }
    elseif (Test-Path -LiteralPath '.env') {
        foreach ($line in Get-Content -LiteralPath '.env' -Encoding UTF8) {
            if ($line -match '^\s*APP_ADDR\s*=\s*(.+)$') { $demoAddr = $Matches[1].Trim().Trim('"').Trim("'") }
        }
    }
    $demoUrl = 'http://' + $demoAddr
    $running = $null
    try { $running = Invoke-RestMethod -Uri ($demoUrl + '/api/health') -TimeoutSec 2 } catch {}
    if ($running.app -ne 'whale-demo') {
        $exePath = Join-Path $PSScriptRoot 'bin\whale-demo.exe'
        if (-not (Test-Path -LiteralPath $exePath)) {
            if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw '没有找到程序，请使用包含 bin 文件夹的完整 Demo，或先安装 Go 编译源码。' }
            New-Item -ItemType Directory -Path (Join-Path $PSScriptRoot 'bin') -Force | Out-Null
            & go build -buildvcs=false -o $exePath .
            if ($LASTEXITCODE -ne 0) { throw '编译没有完成，请查看上方提示。' }
        }
        $logDir = Join-Path $PSScriptRoot 'data'
        New-Item -ItemType Directory -Path $logDir -Force | Out-Null
        $demoProcess = Start-Process -FilePath $exePath -WorkingDirectory $PSScriptRoot -WindowStyle Hidden -RedirectStandardOutput (Join-Path $logDir 'console.log') -RedirectStandardError (Join-Path $logDir 'error.log') -PassThru
        Set-Content -LiteralPath (Join-Path $logDir 'demo.pid') -Value $demoProcess.Id -Encoding ASCII
        $ready = $false
        for ($attempt = 0; $attempt -lt 30; $attempt++) {
            Start-Sleep -Milliseconds 250
            try { $health = Invoke-RestMethod -Uri ($demoUrl + '/api/health') -TimeoutSec 1; if ($health.app -eq 'whale-demo') { $ready = $true; break } } catch {}
            if ($demoProcess.HasExited) { break }
        }
        if (-not $ready) { throw '鲸伴没有启动成功，请检查 8787 端口或 data/error.log。' }
    }
    if (-not $NoBrowser) { Start-Process $demoUrl }
    Write-Host ('鲸伴已启动：' + $demoUrl)
    Write-Host '关闭网页不会停止服务。需要停止时运行 停止鲸伴.cmd。'
} catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}
