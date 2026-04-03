@echo off
chcp 65001 >nul
title 智慧课堂点名提问系统

echo 📦 智慧课堂点名提问系统 - Windows 启动器
echo.

REM Check Python
python --version >nul 2>&1
if errorlevel 1 (
    echo ❌ 未找到 Python，请先从 python.org 安装 Python
    pause
    exit /b 1
)

REM Check Flask
python -c "import flask" >nul 2>&1
if errorlevel 1 (
    echo 📦 正在安装依赖...
    pip install flask
)

REM Create data directory
if not exist "data" mkdir data

REM Kill existing process on port 5000
for /f "tokens=5" %%a in ('netstat -aon ^| findstr :5000 ^| findstr LISTENING') do (
    taskkill /F /PID %%a >nul 2>&1
)

echo 🚀 启动服务...
echo.
echo 📍 教师端: http://127.0.0.1:5000/teacher
echo 📍 学生端: http://127.0.0.1:5000/student
echo.

REM Start in current directory
start http://127.0.0.1:5000/teacher
python rollcall.py
