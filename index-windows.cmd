@echo off
setlocal EnableExtensions
rem Show UTF-8 page titles correctly in this console window.
chcp 65001 >nul 2>&1
title Offline Developer Portal - Indexing
rem Phase 1: crawl the sites listed in sources.yaml into a local corpus.
rem All paths are relative to this script, not the current directory.
set "HERE=%~dp0"
cd /d "%HERE%"

set "APP=%HERE%offline-docs.exe"
if not exist "%APP%" set "APP=%HERE%bin\offline-docs.exe"
if not exist "%APP%" goto :noapp

if exist "%HERE%sources.yaml" goto :run
if exist "%HERE%sources.yaml.txt" goto :txtext

echo.
echo  sources.yaml was not found in:
echo    %HERE%
echo.
echo  sources.yaml lists the documentation websites to index.
echo  A safe starting point is provided in sources.example.yaml.
echo.
choice /C YN /M "Create sources.yaml from sources.example.yaml now and open it in Notepad"
if errorlevel 2 goto :fail
copy /Y "%HERE%sources.example.yaml" "%HERE%sources.yaml" >nul
if errorlevel 1 goto :fail
echo.
echo  Created sources.yaml. Review or edit it in Notepad, save it,
echo  then run index-windows.cmd again.
start "" notepad.exe "%HERE%sources.yaml"
goto :done

:run
"%APP%" index --config "%HERE%sources.yaml" --corpus "%HERE%corpus" %*
if errorlevel 1 goto :fail
echo.
echo  Indexing complete. Next, double-click serve-windows.cmd to open the portal.
goto :done

:noapp
echo.
echo  ERROR: offline-docs.exe was not found next to this script.
echo  Download the Windows release package offline-developer-portal-VERSION-windows-amd64.zip,
echo  extract the WHOLE zip file to a folder, and run this script from that folder.
goto :fail

:txtext
echo.
echo  ERROR: found "sources.yaml.txt" instead of "sources.yaml".
echo  Windows hides known file extensions, so the file got a hidden .txt ending.
echo  Rename it so that it is called exactly sources.yaml and run this script again.
goto :fail

:fail
echo.
echo  Indexing did not complete. Read the messages above for details.
echo.
pause
endlocal
exit /b 1

:done
echo.
pause
endlocal
exit /b 0
