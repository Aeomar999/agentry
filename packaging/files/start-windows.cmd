@echo off
rem Starts Agentry and opens it in your browser. Run it again any time to
rem open the page; Agentry keeps running when you close the window.
cd /d "%~dp0"
agentry.exe open
timeout /t 10 >nul
