#!/bin/bash
# 简化版Kronos设置（仅下载模型，不克隆GitHub项目）

set -e

echo "============================================================"
echo "Kronos模型下载（简化版）"
echo "============================================================"
echo ""

# 检查Python环境
if ! command -v python3 &> /dev/null; then
    echo "❌ Python3未安装"
    exit 1
fi

# 安装依赖
echo "安装Python依赖..."
pip3 install -q torch transformers huggingface_hub pandas numpy safetensors

# 创建模型目录
MODEL_DIR="models/kronos"
mkdir -p "$MODEL_DIR"

# 下载模型和tokenizer
echo ""
echo "下载Kronos模型和Tokenizer..."
python3 << 'PYTHON_SCRIPT'
import os
from huggingface_hub import snapshot_download
from pathlib import Path

model_dir = Path("models/kronos")
model_dir.mkdir(parents=True, exist_ok=True)

print("正在下载Kronos-base模型（这可能需要几分钟）...")
try:
    model_path = snapshot_download(
        repo_id="NeoQuasar/Kronos-base",
        local_dir=str(model_dir / "Kronos-base"),
        local_dir_use_symlinks=False
    )
    print(f"✓ 模型下载完成: {model_path}")
except Exception as e:
    print(f"❌ 模型下载失败: {e}")
    print("   请检查网络连接或稍后重试")
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
    print("   请检查网络连接或稍后重试")
    exit(1)

print("\n============================================================")
print("✓ 模型下载完成")
print(f"模型目录: {model_dir.absolute()}")
print("")
print("⚠️  注意: Kronos需要自定义模型类才能使用")
print("   请克隆Kronos GitHub项目到 ../etg_ai 目录")
print("   或运行: bash scripts/setup_kronos_complete.sh")
print("============================================================")
PYTHON_SCRIPT

echo ""
echo "✓ 下载完成！"

