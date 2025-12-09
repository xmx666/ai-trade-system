#!/bin/bash
# 下载Kronos模型和Tokenizer

set -e

echo "============================================================"
echo "下载Kronos模型和Tokenizer"
echo "============================================================"
echo ""

# 检查Python环境
if ! command -v python3 &> /dev/null; then
    echo "❌ Python3未安装"
    exit 1
fi

# 安装依赖
echo "安装依赖..."
pip3 install -q torch transformers huggingface_hub pandas numpy safetensors

# 创建模型目录
MODEL_DIR="../models/kronos"
mkdir -p "$MODEL_DIR"

echo ""
echo "开始下载模型..."
echo ""

# 下载模型和tokenizer
python3 << 'PYTHON_SCRIPT'
import os
from huggingface_hub import snapshot_download
from pathlib import Path

model_dir = Path("../models/kronos")
model_dir.mkdir(parents=True, exist_ok=True)

print("正在下载Kronos-base模型...")
try:
    model_path = snapshot_download(
        repo_id="NeoQuasar/Kronos-base",
        local_dir=str(model_dir / "Kronos-base"),
        local_dir_use_symlinks=False
    )
    print(f"✓ 模型下载完成: {model_path}")
except Exception as e:
    print(f"❌ 模型下载失败: {e}")
    exit(1)

print("\n正在下载Kronos-Tokenizer-base...")
try:
    tokenizer_path = snapshot_download(
        repo_id="NeoQuasar/Kronos-Tokenizer-base",
        local_dir=str(model_dir / "Kronos-Tokenizer-base"),
        local_dir_use_symlinks=False
    )
    print(f"✓ Tokenizer下载完成: {tokenizer_path}")
except Exception as e:
    print(f"❌ Tokenizer下载失败: {e}")
    exit(1)

print("\n============================================================")
print("✓ 所有文件下载完成")
print(f"模型目录: {model_dir}")
print("============================================================")
PYTHON_SCRIPT

echo ""
echo "下载完成！"

