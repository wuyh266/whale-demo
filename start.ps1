param([switch]$NoBrowser)
$ErrorActionPreference = 'Stop'
try {
    Set-Location -LiteralPath $PSScriptRoot
    # Keep build caches and temporary files beside the demo.
    $runtimeDir = Join-Path $PSScriptRoot 'data\build-cache'
    New-Item -ItemType Directory -Path $runtimeDir -Force | Out-Null
    $env:GOCACHE = $runtimeDir
    $env:TEMP = Join-Path $runtimeDir 'tmp'
    $env:TMP = $env:TEMP
    New-Item -ItemType Directory -Path $env:TEMP -Force | Out-Null
    $launchConfig = @{}
    if (Test-Path -LiteralPath '.env') {
        foreach ($line in Get-Content -LiteralPath '.env' -Encoding UTF8) {
            if ($line -match '^\s*([A-Z_]+)\s*=\s*(.*)$') { $launchConfig[$Matches[1]] = $Matches[2].Trim().Trim('"').Trim("'") }
        }
    }
    foreach ($key in @('TTS_PROVIDER', 'GPT_SOVITS_API_BASE', 'GPT_SOVITS_PROFILE', 'APP_ADDR', 'DATA_DIR')) {
        $override = [Environment]::GetEnvironmentVariable($key)
        if ($override) { $launchConfig[$key] = $override }
    }
    $startLocalVoice = $launchConfig['TTS_PROVIDER'] -eq 'gpt-sovits'
    if (-not $startLocalVoice) {
        $stateDirectory = if ($launchConfig['DATA_DIR']) { $launchConfig['DATA_DIR'] } else { 'data' }
        if (-not [IO.Path]::IsPathRooted($stateDirectory)) { $stateDirectory = Join-Path $PSScriptRoot $stateDirectory }
        $statePath = Join-Path $stateDirectory 'state.json'
        try {
            if (Test-Path -LiteralPath $statePath) {
                $savedState = Get-Content -LiteralPath $statePath -Raw -Encoding UTF8 | ConvertFrom-Json
                $localVoiceCompanions = @($savedState.companions | Where-Object { $_.voice -in @('anon', 'soyo', 'sakiko_white') })
                $hasAnonTemplate = @($savedState.companions | Where-Object { $_.id -eq 'template-anon' }).Count -gt 0
                $startLocalVoice = (-not $hasAnonTemplate) -or $localVoiceCompanions.Count -gt 0
            } else { $startLocalVoice = $true }
        } catch { Write-Warning '未能读取伴侣声音偏好，启动后可单独运行 START-SOVITS.cmd。' }
    }
    if ($startLocalVoice) {
        $ttsBase = if ($launchConfig['GPT_SOVITS_API_BASE']) { $launchConfig['GPT_SOVITS_API_BASE'] } else { 'http://127.0.0.1:9880' }
        $ttsProfile = if ($launchConfig['GPT_SOVITS_PROFILE']) { $launchConfig['GPT_SOVITS_PROFILE'] } else { 'config\sovits.json' }
        if ($ttsBase.TrimEnd('/') -eq 'http://127.0.0.1:9880' -and (Test-Path -LiteralPath $ttsProfile)) {
            & powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'start-sovits.ps1') -Profile $ttsProfile -WaitSeconds 10
            if ($LASTEXITCODE -ne 0) { Write-Warning '本地语音未就绪，文字聊天仍可使用；请检查 data/sovits-error.log。' }
        }
    }
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
        $needsBuild = -not (Test-Path -LiteralPath $exePath)
        if (-not $needsBuild) {
            $binaryTime = (Get-Item -LiteralPath $exePath).LastWriteTimeUtc
            $sourceFiles = @(Get-ChildItem -LiteralPath $PSScriptRoot -Filter '*.go' -File) + @(Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'web') -Recurse -File)
            $needsBuild = @($sourceFiles | Where-Object { $_.LastWriteTimeUtc -gt $binaryTime }).Count -gt 0
        }
        if ($needsBuild) {
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
