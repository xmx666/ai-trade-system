#!/bin/bash
# 完整设置Kronos模型（使用代理）

set -e

cd "$(dirname "$0")/.."

echo "============================================================"
echo "Kronos模型完整设置（使用代理）"
echo "============================================================"
echo ""

# 读取代理配置
PROXY_CONFIG_FILE=".env"
if [ -f "$PROXY_CONFIG_FILE" ]; then
    echo "从 .env 文件读取代理配置..."
    source <(grep -E "^HTTP_PROXY=|^HTTPS_PROXY=" "$PROXY_CONFIG_FILE" | sed 's/^/export /')
    echo "HTTP_PROXY: ${HTTP_PROXY:-未设置}"
    echo "HTTPS_PROXY: ${HTTPS_PROXY:-未设置}"
else
    echo "未找到 .env 文件，使用环境变量中的代理配置"
fi

# 设置代理环境变量
if [ -n "$HTTP_PROXY" ]; then
    export HTTP_PROXY
    export http_proxy="$HTTP_PROXY"
fi
if [ -n "$HTTPS_PROXY" ]; then
    export HTTPS_PROXY
    export https_proxy="$HTTPS_PROXY"
fi

echo ""

# 步骤1: 下载模型
echo "步骤1: 下载Kronos模型..."
bash scripts/download_kronos_with_proxy.sh

echo ""

# 步骤2: 获取代码
echo "步骤2: 获取Kronos代码文件..."
bash scripts/get_kronos_code_with_proxy.sh

echo ""

# 步骤3: 验证
echo "步骤3: 验证安装..."
bash scripts/check_kronos_status.sh

echo ""
echo "============================================================"
echo "✓ Kronos模型设置完成！"
echo "============================================================"

