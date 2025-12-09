#!/bin/bash
# 完整设置Kronos模型（包括GitHub项目和模型下载）

set -e

echo "============================================================"
echo "完整设置Kronos模型"
echo "============================================================"
echo ""

# 检查Python环境
if ! command -v python3 &> /dev/null; then
    echo "❌ Python3未安装"
    exit 1
fi

# 安装基础依赖
echo "步骤1: 安装Python依赖..."
pip3 install -q torch transformers huggingface_hub pandas numpy safetensors

# 检查Kronos GitHub项目（可选）
ETG_AI_DIR="../etg_ai"
echo ""
echo "步骤2: 检查Kronos GitHub项目（可选）..."
if [ ! -d "$ETG_AI_DIR" ]; then
    echo "⚠️  Kronos GitHub项目未找到"
    echo "   可以从Hugging Face直接使用模型，无需GitHub项目"
    echo "   如果需要GitHub项目，请手动克隆到 ../etg_ai 目录"
    echo "   参考: https://huggingface.co/NeoQuasar/Kronos-base"
else
    echo "✓ Kronos项目已存在"
    # 安装Kronos项目依赖
    if [ -f "$ETG_AI_DIR/requirements.txt" ]; then
        echo "安装Kronos项目依赖..."
        cd "$ETG_AI_DIR"
        pip3 install -q -r requirements.txt
        cd - > /dev/null
        echo "✓ 依赖安装完成"
    fi
fi

# 创建模型目录
MODEL_DIR="models/kronos"
mkdir -p "$MODEL_DIR"

# 下载模型和tokenizer
echo ""
echo "步骤4: 下载Kronos模型和Tokenizer..."
python3 << 'PYTHON_SCRIPT'
import os
from huggingface_hub import snapshot_download
from pathlib import Path

model_dir = Path("models/kronos")
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
    print(f"⚠️  模型下载失败: {e}")
    print("   可以稍后手动下载或使用在线版本")

print("\n正在下载Kronos-Tokenizer-base...")
try:
    tokenizer_path = snapshot_download(
        repo_id="NeoQuasar/Kronos-Tokenizer-base",
        local_dir=str(model_dir / "Kronos-Tokenizer-base"),
        local_dir_use_symlinks=False
    )
    print(f"✓ Tokenizer下载完成: {tokenizer_path}")
except Exception as e:
    print(f"⚠️  Tokenizer下载失败: {e}")
    print("   可以稍后手动下载或使用在线版本")

print("\n============================================================")
print("✓ 设置完成")
print(f"项目目录: {Path('../etg_ai').absolute()}")
print(f"模型目录: {model_dir.absolute()}")
print("============================================================")
PYTHON_SCRIPT

echo ""
echo "✓ Kronos模型设置完成！"
echo ""
echo "下一步："
echo "1. 确保模型文件已下载到 models/kronos/ 目录"
echo "2. 运行测试: cd predictor && python3 test_krnos_direct.py"

