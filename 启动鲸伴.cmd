@echo off
setlocal
cd /d "%~dp0"
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0start.ps1" %*
set "WHALE_LAUNCH_EXIT=%ERRORLEVEL%"
if not "%WHALE_LAUNCH_EXIT%"=="0" pause
exit /b %WHALE_LAUNCH_EXIT%
