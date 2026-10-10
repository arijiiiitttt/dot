@echo off
REM Builds Dot for 64-bit and 32-bit Windows into bin64\ and bin32\.
REM Use Go 1.20.x if the exe must run on Windows 7/8 (Go 1.21+ needs Windows 10).
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags="-s -w" -o bin64\dot.exe . || goto :err
set GOARCH=386
go build -trimpath -ldflags="-s -w" -o bin32\dot.exe . || goto :err
echo Done: bin64\dot.exe  bin32\dot.exe
exit /b 0
:err
echo Build failed. Is Go installed? (go version)
exit /b 1
