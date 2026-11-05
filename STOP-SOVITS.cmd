@echo off
setlocal
cd /d "%~dp0"
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0stop-sovits.ps1" %*
set "WHALE_SOVITS_EXIT=%ERRORLEVEL%"
if not "%WHALE_SOVITS_EXIT%"=="0" pause
exit /b %WHALE_SOVITS_EXIT%
