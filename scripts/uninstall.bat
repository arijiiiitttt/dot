@echo off
set "DEST=%LOCALAPPDATA%\Dot"
powershell -NoProfile -ExecutionPolicy Bypass -Command "$d='%DEST%'; $p=[Environment]::GetEnvironmentVariable('Path','User'); if($p){[Environment]::SetEnvironmentVariable('Path',(($p -split ';' | ? {$_ -and $_ -ne $d}) -join ';'),'User')}"
del /Q "%DEST%\dot.exe" "%DEST%\.dot.exe" 2>nul
rmdir "%DEST%" 2>nul
reg delete "HKCU\Software\Microsoft\Windows NT\CurrentVersion\Fonts" /v "DotIcons (TrueType)" /f >nul 2>nul
echo Dot removed (the icon font file can be deleted from Windows Settings ^> Fonts).
pause
