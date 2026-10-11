@echo off
REM Installs Dot on Windows XP (no PowerShell / %LOCALAPPDATA% / setx needed). Also works on newer Windows.
setlocal
set "ROOT=%~dp0.."
set "DEST=%USERPROFILE%\Dot"
if not exist "%ROOT%\binxp\dot.exe" (echo Build first: scripts\build_xp.bat & pause & exit /b 1)
if not exist "%DEST%" mkdir "%DEST%"
copy /Y "%ROOT%\binxp\dot.exe" "%DEST%\dot.exe" >nul
copy /Y "%ROOT%\binxp\dot.exe" "%DEST%\.dot.exe" >nul
REM add to the user PATH (registry)
set "UP="
for /f "tokens=2,*" %%a in ('reg query "HKCU\Environment" /v Path 2^>nul ^| find "REG_"') do set "UP=%%b"
echo ;%UP%; | find /I ";%DEST%;" >nul
if errorlevel 1 (
  if defined UP (set "NP=%UP%;%DEST%") else (set "NP=%DEST%")
  reg add "HKCU\Environment" /v Path /t REG_EXPAND_SZ /d "%NP%" /f >nul
  echo  Added %DEST% to your PATH.
)
echo.
echo  Done. Log off and on again (XP needs this), open cmd and type:  .dot
echo  Until then you can run:  "%DEST%\dot.exe"
pause
