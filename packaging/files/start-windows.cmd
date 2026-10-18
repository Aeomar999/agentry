@echo off
rem Starts Agentry and opens it in your browser. Run it again any time to
rem open the page; Agentry keeps running when you close the window. It
rem waits for a key, so you can read or copy the links first.
cd /d "%~dp0"
agentry.exe open
pause
