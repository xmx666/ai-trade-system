#!/bin/bash
# krnos预测服务测试脚本（支持代理配置）

set -e

echo "============================================================"
echo "krnos预测服务测试（支持代理）"
echo "============================================================"
echo ""

# 检查并读取代理配置
PROXY_CONFIG_FILE="../.env"
if [ -f "$PROXY_CONFIG_FILE" ]; then
    echo "从 .env 文件读取代理配置..."
    source <(grep -E "^HTTP_PROXY=|^HTTPS_PROXY=" "$PROXY_CONFIG_FILE" | sed 's/^/export /')
    echo "HTTP_PROXY: ${HTTP_PROXY:-未设置}"
    echo "HTTPS_PROXY: ${HTTPS_PROXY:-未设置}"
else
    echo "未找到 .env 文件，使用环境变量中的代理配置"
    echo "HTTP_PROXY: ${HTTP_PROXY:-未设置}"
    echo "HTTPS_PROXY: ${HTTPS_PROXY:-未设置}"
fi

echo ""

# 如果未设置代理，尝试从系统环境变量读取
if [ -z "$HTTP_PROXY" ] && [ -z "$HTTPS_PROXY" ]; then
    echo "⚠️  未检测到代理配置"
    echo "   如果使用VPN，请设置HTTP_PROXY和HTTPS_PROXY环境变量"
    echo "   例如: export HTTP_PROXY=http://127.0.0.1:7890"
    echo ""
fi

# 设置代理环境变量（传递给Python）
if [ -n "$HTTP_PROXY" ]; then
    export HTTP_PROXY
fi
if [ -n "$HTTPS_PROXY" ]; then
    export HTTPS_PROXY
fi

# 检查Python依赖
echo "检查Python依赖..."
if ! python3 -c "import requests" 2>/dev/null; then
    echo "安装 requests..."
    pip3 install requests --quiet
fi
if ! python3 -c "import numpy" 2>/dev/null; then
    echo "安装 numpy..."
    pip3 install numpy --quiet
fi
echo "✓ 依赖检查完成"
echo ""

# 运行测试
echo "============================================================"
echo "开始测试"
echo "============================================================"
echo ""

python3 test_krnos_direct.py

echo ""
echo "============================================================"
echo "测试完成"
echo "============================================================"

