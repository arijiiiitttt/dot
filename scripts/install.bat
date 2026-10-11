@echo off
setlocal
set "ROOT=%~dp0.."
title Install Dot
set "DEST=%LOCALAPPDATA%\Dot"
set "ARCH=64"
if /I "%PROCESSOR_ARCHITECTURE%"=="x86" if not defined PROCESSOR_ARCHITEW6432 set "ARCH=32"
echo.
echo  Installing Dot (%ARCH%-bit) to %DEST%
if not exist "%DEST%" mkdir "%DEST%"
copy /Y "%ROOT%\bin%ARCH%\dot.exe" "%DEST%\dot.exe" >nul
if errorlevel 1 (echo  Copy failed - run scripts\build.bat first. & pause & exit /b 1)
copy /Y "%ROOT%\bin%ARCH%\dot.exe" "%DEST%\.dot.exe" >nul
powershell -NoProfile -ExecutionPolicy Bypass -Command "$d='%DEST%'; $p=[Environment]::GetEnvironmentVariable('Path','User'); if(-not $p){$p=''}; if(($p -split ';') -notcontains $d){[Environment]::SetEnvironmentVariable('Path',($p.TrimEnd(';')+';'+$d).TrimStart(';'),'User'); Write-Host ' Added to your PATH.'} else {Write-Host ' Already on PATH.'}"
REM ---- file-icon font (Windows 10 1809+ per-user install) ----
set "FD=%LOCALAPPDATA%\Microsoft\Windows\Fonts"
if not exist "%FD%" mkdir "%FD%" 2>nul
copy /Y "%ROOT%\assets\font\DotIcons.ttf" "%FD%\DotIcons.ttf" >nul 2>nul
if not errorlevel 1 reg add "HKCU\Software\Microsoft\Windows NT\CurrentVersion\Fonts" /v "DotIcons (TrueType)" /t REG_SZ /d "%FD%\DotIcons.ttf" /f >nul 2>nul
echo.
echo  Done!  Close this window, open a NEW cmd / PowerShell / Windows Terminal and type:
echo.
echo        .dot
echo.
echo  FILE ICONS: in Windows Terminal open Settings ^> Profiles ^> Defaults ^> Appearance
echo  and set  Font face  to:   Cascadia Mono, DotIcons
echo  (If icons show as squares, right-click assets\font\DotIcons.ttf ^> Install, then restart the terminal.)
echo.
pause
