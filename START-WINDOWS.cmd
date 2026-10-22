@echo off
cd /d "%~dp0"
if not exist config.json copy config.example.json config.json >nul
if exist ness-unified.exe (
  ness-unified.exe
) else (
  go run .
)
pause
