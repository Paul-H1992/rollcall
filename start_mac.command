#!/bin/bash
cd "$(dirname "$0")"

echo "📦 智慧课堂点名提问系统 - macOS 启动器"
echo ""

# Check Python
if ! command -v python3 &> /dev/null; then
    echo "❌ 未找到 Python3，请先从 python.org 安装 Python"
    echo "按任意键退出..."
    read
    exit 1
fi

# Check Flask
if ! python3 -c "import flask" &> /dev/null; then
    echo "📦 正在安装依赖..."
    pip3 install flask
fi

# Create data directory
mkdir -p data

# Kill existing process on port 8080
lsof -ti :8080 | xargs kill -9 2>/dev/null || true

echo "🚀 启动服务..."
echo ""
open "http://127.0.0.1:8080/teacher"

python3 rollcall.py --port 8080
