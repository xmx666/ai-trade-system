#!/bin/bash
# 使用代理下载Kronos模型

set -e

cd "$(dirname "$0")/.."

echo "============================================================"
echo "使用代理下载Kronos模型"
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
    echo "HTTP_PROXY: ${HTTP_PROXY:-未设置}"
    echo "HTTPS_PROXY: ${HTTPS_PROXY:-未设置}"
fi

echo ""

# 设置代理环境变量
if [ -n "$HTTP_PROXY" ]; then
    export HTTP_PROXY
    export http_proxy="$HTTP_PROXY"
fi
if [ -n "$HTTPS_PROXY" ]; then
    export HTTPS_PROXY
    export https_proxy="$HTTPS_PROXY"
fi

# 创建目录
mkdir -p models/kronos

# 下载模型
echo "开始下载模型..."
python3 << 'PYTHON'
import os
import sys
from huggingface_hub import snapshot_download
from pathlib import Path

# 配置代理
http_proxy = os.environ.get('HTTP_PROXY') or os.environ.get('http_proxy')
https_proxy = os.environ.get('HTTPS_PROXY') or os.environ.get('https_proxy')

if http_proxy or https_proxy:
    print(f"使用代理: HTTP={http_proxy}, HTTPS={https_proxy}")
    # huggingface_hub会自动使用环境变量中的代理

model_dir = Path("models/kronos")
model_dir.mkdir(parents=True, exist_ok=True)

# 下载模型
if not (model_dir / "Kronos-base").exists():
    print("\n正在下载Kronos-base模型（这可能需要几分钟）...")
    try:
        snapshot_download(
            "NeoQuasar/Kronos-base",
            local_dir=str(model_dir / "Kronos-base"),
            local_dir_use_symlinks=False
        )
        print("✓ Kronos-base模型下载完成")
    except Exception as e:
        print(f"❌ 模型下载失败: {e}")
        print("   请检查:")
        print("   1. 网络连接")
        print("   2. 代理配置是否正确")
        print("   3. VPN是否正常工作")
        sys.exit(1)
else:
    print("✓ Kronos-base模型已存在")

# 下载Tokenizer
if not (model_dir / "Kronos-Tokenizer-base").exists():
    print("\n正在下载Kronos-Tokenizer-base...")
    try:
        snapshot_download(
            "NeoQuasar/Kronos-Tokenizer-base",
            local_dir=str(model_dir / "Kronos-Tokenizer-base"),
            local_dir_use_symlinks=False
        )
        print("✓ Kronos-Tokenizer-base下载完成")
    except Exception as e:
        print(f"❌ Tokenizer下载失败: {e}")
        sys.exit(1)
else:
    print("✓ Kronos-Tokenizer-base已存在")

print("\n" + "=" * 60)
print("✓ 所有模型文件下载完成")
print("=" * 60)
PYTHON

echo ""
echo "✓ 下载完成！"

