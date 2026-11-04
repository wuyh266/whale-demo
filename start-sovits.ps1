param(
    [string]$Profile = 'config\sovits.json',
    [ValidateRange(1, 65535)][int]$Port = 9880,
    [ValidateRange(5, 600)][int]$WaitSeconds = 45
)
$ErrorActionPreference = 'Stop'
function Resolve-WhaleLocalPath {
    param([Parameter(Mandatory=$true)][string]$Value, [string]$Base = $PSScriptRoot)
    if ([string]::IsNullOrWhiteSpace($Value)) { throw '资源路径不能为空。' }
    $demoRoot = [IO.Path]::GetFullPath($PSScriptRoot).TrimEnd([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
    $candidate = if ([IO.Path]::IsPathRooted($Value)) { $Value } else { Join-Path $Base $Value }
    $fullPath = [IO.Path]::GetFullPath($candidate)
    if (-not ($fullPath.Equals($demoRoot, [StringComparison]::OrdinalIgnoreCase) -or $fullPath.StartsWith($demoRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase))) {
        throw ('资源必须位于 whale-demo 文件夹内：' + $fullPath)
    }
    # A lexical prefix alone is insufficient when a directory is a junction.
    # Check the target and all existing ancestors before reading or executing it.
    $cursor = $fullPath
    while ($cursor) {
        if (Test-Path -LiteralPath $cursor) {
            $entry = Get-Item -LiteralPath $cursor -Force
            if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw ('资源路径不能经过符号链接或目录联接：' + $cursor)
            }
        }
        $parent = [IO.Path]::GetDirectoryName($cursor)
        if (-not $parent -or $parent -eq $cursor) { break }
        $cursor = $parent
    }
    return $fullPath
}

try {
    $profilePath = Resolve-WhaleLocalPath -Value $Profile
    if (-not (Test-Path -LiteralPath $profilePath -PathType Leaf)) { throw ('未找到语音配置：' + $profilePath) }
    $sovitsProfile = Get-Content -LiteralPath $profilePath -Raw -Encoding UTF8 | ConvertFrom-Json
    $sourcePath = Resolve-WhaleLocalPath -Value $sovitsProfile.source_dir
    if (-not (Test-Path -LiteralPath $sourcePath -PathType Container)) { throw ('语音后端不完整，请保留整个 backend 文件夹：' + $sourcePath) }
    $pythonPath = Resolve-WhaleLocalPath -Value $sovitsProfile.python -Base $sourcePath
    if (-not (Test-Path -LiteralPath $pythonPath -PathType Leaf)) { throw ('未找到随包提供的 GPT-SoVITS Python：' + $pythonPath) }
    $bridgePath = Resolve-WhaleLocalPath -Value 'tools\sovits_server.py'
    $dataDir = Resolve-WhaleLocalPath -Value 'data'
    New-Item -ItemType Directory -Path $dataDir -Force | Out-Null
    $processFile = Resolve-WhaleLocalPath -Value 'data\sovits-process.json'
    $record = $null
    $existing = $null
    $ownedProcess = $false
    if (Test-Path -LiteralPath $processFile -PathType Leaf) {
        $record = Get-Content -LiteralPath $processFile -Raw -Encoding UTF8 | ConvertFrom-Json
        $existing = Get-Process -Id ([int]$record.pid) -ErrorAction SilentlyContinue
        $ownedProcess = $existing -and $existing.Path -eq $pythonPath -and $record.bridge -eq $bridgePath -and $record.startedAtUtcTicks -eq $existing.StartTime.ToUniversalTime().Ticks.ToString()
        if ($ownedProcess -and [int]$record.port -ne $Port) { throw '本地语音服务已在其他端口启动，请先运行 STOP-SOVITS.cmd 再更换端口。' }
        if ($ownedProcess -and $record.profile -ne $profilePath) { throw '本地语音服务使用了其他配置，请先运行 STOP-SOVITS.cmd 再切换配置。' }
    }
    $url = 'http://127.0.0.1:' + $Port
    try { $health = Invoke-RestMethod -Uri ($url + '/health') -TimeoutSec 2 } catch { $health = $null }
    if ($health.app -eq 'whale-sovits') {
        if (-not $ownedProcess) { throw '语音端口已由其他语音进程占用，无法确认它使用随包后端；请先停止旧服务，再运行本目录的 START-SOVITS.cmd。' }
        Write-Host ('本地语音服务已就绪：' + $url)
        exit 0
    }
    if ($ownedProcess) {
        Write-Host '本地语音进程已启动，可能仍在加载模型，请查看 data/sovits-console.log。'
        exit 0
    }
    $arguments = '-B "' + $bridgePath + '" --profile "' + $profilePath + '" --port ' + $Port
    $sovitsProcess = Start-Process -FilePath $pythonPath -ArgumentList $arguments -WorkingDirectory $PSScriptRoot -WindowStyle Hidden -RedirectStandardOutput (Join-Path $dataDir 'sovits-console.log') -RedirectStandardError (Join-Path $dataDir 'sovits-error.log') -PassThru
    @{ pid = $sovitsProcess.Id; python = $pythonPath; bridge = $bridgePath; profile = $profilePath; source = $sourcePath; port = $Port; startedAtUtcTicks = $sovitsProcess.StartTime.ToUniversalTime().Ticks.ToString() } | ConvertTo-Json | Set-Content -LiteralPath $processFile -Encoding UTF8
    $deadline = (Get-Date).AddSeconds($WaitSeconds)
    while ((Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 500
        $sovitsProcess.Refresh()
        if ($sovitsProcess.HasExited) { throw '语音模型未能启动，请检查 data/sovits-error.log；whale 文字聊天仍可使用。' }
        try { $health = Invoke-RestMethod -Uri ($url + '/health') -TimeoutSec 1; if ($health.app -eq 'whale-sovits') { Write-Host ('本地语音服务已就绪：' + $url); exit 0 } } catch {}
    }
    Write-Host '语音进程仍在加载模型，首次启动可能较慢；就绪后可点“试听当前音色”。日志：data/sovits-console.log'
    exit 0
} catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}
