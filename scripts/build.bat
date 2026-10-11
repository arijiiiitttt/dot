@echo off
cd /d "%~dp0.."
set CGO_ENABLED=0
set GOOS=windows

echo [1/3] native dot.exe
go build -trimpath -ldflags="-s -w" -o dot.exe . || goto :err

echo [2/3] bin64\dot.exe
set GOARCH=amd64
go build -trimpath -ldflags="-s -w" -o bin64\dot.exe . || goto :err

echo [3/3] bin32\dot.exe
set GOARCH=386
go build -trimpath -ldflags="-s -w" -o bin32\dot.exe . || goto :err

echo.
echo Done:  dot.exe   bin64\dot.exe   bin32\dot.exe
exit /b 0

:err
echo Build failed.
exit /b 1