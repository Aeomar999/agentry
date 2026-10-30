@echo off
rem Stops Agentry.
cd /d "%~dp0"
agentry.exe stop
timeout /t 5 >nul
