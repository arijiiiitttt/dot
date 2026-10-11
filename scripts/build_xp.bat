@echo off
REM Builds the Windows XP-compatible exe  ->  binxp\dot.exe  (32-bit, runs on XP SP2 ... Windows 11)
REM Needs Go 1.10.x (the last Go that supports XP):  https://go.dev/dl/go1.10.8.windows-386.zip
REM Go 1.10 has no modules, so the source is copied into a temporary GOPATH first.
setlocal
cd /d "%~dp0.."
go version | find "go1.10" >nul
if errorlevel 1 (
  echo This build needs Go 1.10.x. You have:
  go version
  echo Download go1.10.8 and put its bin folder first in PATH, then run this again.
  exit /b 1
)
set "GP=%TEMP%\dotgp"
if exist "%GP%" rmdir /s /q "%GP%"
mkdir "%GP%\src\dot"
xcopy /E /I /Q /Y main.go "%GP%\src\dot\" >nul
xcopy /E /I /Q /Y src "%GP%\src\dot\src\" >nul
set GOPATH=%GP%
set GO111MODULE=off
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=386
if not exist binxp mkdir binxp
go build -ldflags="-s -w" -o "%CD%\binxp\dot.exe" dot || goto :err
echo Done: binxp\dot.exe
exit /b 0
:err
echo Build failed.
exit /b 1
