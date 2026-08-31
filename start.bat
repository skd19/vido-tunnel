@echo off
cd /d "%~dp0"
start "" "vido-tunnel.exe" -root "C:\Users\sk\Videos\Vidoveo" -key "yourSecretKey123" -port 8080 -vidoveo-path "C:\Vidoveo\Vidoveo.exe" -vidoveo-port 7788
exit
