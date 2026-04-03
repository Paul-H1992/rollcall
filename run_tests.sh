#!/bin/bash
# 运行点名系统自动化测试
# 用法: bash run_tests.sh

set -e

echo "=== 智慧课堂点名系统 测试脚本 ==="
echo ""

# 检查服务是否运行
echo "[1/3] 检查服务状态..."
if curl -s http://127.0.0.1:8888/api/stats > /dev/null 2>&1; then
    echo "✅ 服务正在运行 (http://127.0.0.1:8888)"
else
    echo "❌ 服务未启动!"
    echo "请先启动服务: python3 rollcall.py --port 8888"
    exit 1
fi

# 检查pytest
echo "[2/3] 检查测试依赖..."
if ! command -v pytest &> /dev/null; then
    echo "安装 pytest..."
    pip3 install pytest requests -q
fi
echo "✅ pytest 已就绪"

# 运行测试
echo "[3/3] 运行测试..."
echo ""

cd "$(dirname "$0")"
pytest tests/test_api.py -v --tb=short

echo ""
echo "=== 测试完成 ==="
