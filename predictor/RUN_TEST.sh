#!/bin/bash
# krnos预测服务测试脚本
# 自动运行所有测试并生成报告

set -e

echo "============================================================"
echo "krnos预测服务测试"
echo "============================================================"
echo ""

# 检查Python环境
echo "检查Python环境..."
if ! command -v python3 &> /dev/null; then
    echo "❌ Python3 未安装"
    exit 1
fi
echo "✓ Python3 已安装"

# 检查依赖
echo ""
echo "检查Python依赖..."
if ! python3 -c "import requests" 2>/dev/null; then
    echo "安装 requests 库..."
    pip3 install requests --quiet
fi
echo "✓ requests 已安装"

if ! python3 -c "import numpy" 2>/dev/null; then
    echo "安装 numpy 库..."
    pip3 install numpy --quiet
fi
echo "✓ numpy 已安装"

# 运行测试
echo ""
echo "============================================================"
echo "开始测试"
echo "============================================================"
echo ""

# 方式1: 尝试使用Go程序获取数据
if command -v go &> /dev/null; then
    echo "方式1: 使用Go程序获取真实数据"
    if go run test_krnos_real.go 2>&1; then
        echo ""
        echo "运行Python预测测试..."
        python3 test_krnos.py
        exit 0
    else
        echo "⚠️  Go程序失败，尝试方式2"
    fi
fi

# 方式2: 使用Python直接获取数据
echo ""
echo "方式2: 使用Python直接获取真实数据"
python3 test_krnos_direct.py

echo ""
echo "============================================================"
echo "测试完成"
echo "============================================================"

