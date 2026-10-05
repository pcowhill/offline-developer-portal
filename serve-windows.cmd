@echo off
setlocal EnableExtensions
rem Show UTF-8 page titles correctly in this console window.
chcp 65001 >nul 2>&1
title Offline Developer Portal
rem Phase 2: serve the portal and the generated corpus on this computer only.
rem No Internet access is needed. Close this window or press Ctrl+C to stop.
set "HERE=%~dp0"
cd /d "%HERE%"

set "APP=%HERE%offline-docs.exe"
if not exist "%APP%" set "APP=%HERE%bin\offline-docs.exe"
if not exist "%APP%" goto :noapp

if not exist "%HERE%corpus\manifest.json" (
  echo.
  echo  NOTE: no index found yet. Run index-windows.cmd first;
  echo  until then the portal only shows setup instructions.
)

"%APP%" serve --dist "%HERE%dist" --corpus "%HERE%corpus" --open %*
if errorlevel 1 goto :fail
endlocal
exit /b 0

:noapp
echo.
echo  ERROR: offline-docs.exe was not found next to this script.
echo  Download the Windows release package offline-developer-portal-VERSION-windows-amd64.zip,
echo  extract the WHOLE zip file to a folder, and run this script from that folder.

:fail
echo.
echo  The portal server stopped with an error. Read the messages above for details.
echo.
pause
endlocal
exit /b 1
