#!/bin/bash
# 检查Kronos设置状态

cd "$(dirname "$0")/.."

echo "============================================================"
echo "Kronos模型设置状态检查"
echo "============================================================"
echo ""

# 检查模型文件
echo "1. 模型文件:"
MODEL_DIR="models/kronos"
if [ -d "$MODEL_DIR/Kronos-base" ]; then
    SIZE=$(du -sh "$MODEL_DIR/Kronos-base" 2>/dev/null | cut -f1)
    echo "   ✓ Kronos-base模型存在 (大小: $SIZE)"
else
    echo "   ❌ Kronos-base模型不存在"
fi

if [ -d "$MODEL_DIR/Kronos-Tokenizer-base" ]; then
    SIZE=$(du -sh "$MODEL_DIR/Kronos-Tokenizer-base" 2>/dev/null | cut -f1)
    echo "   ✓ Kronos-Tokenizer-base存在 (大小: $SIZE)"
else
    echo "   ❌ Kronos-Tokenizer-base不存在"
fi

echo ""

# 检查代码文件
echo "2. 代码文件:"
ETG_AI="../etg_ai"
if [ -f "$ETG_AI/model.py" ]; then
    echo "   ✓ model.py存在"
else
    echo "   ❌ model.py不存在"
fi

if [ -f "$ETG_AI/tokenizer.py" ]; then
    echo "   ✓ tokenizer.py存在"
else
    echo "   ❌ tokenizer.py不存在"
fi

echo ""

# 测试导入
echo "3. 测试导入:"
python3 << 'PYTHON'
import sys
from pathlib import Path

# 测试代码导入
etg_ai = Path("../etg_ai")
if etg_ai.exists():
    sys.path.insert(0, str(etg_ai.absolute()))
    try:
        from model import Kronos, KronosTokenizer
        print("   ✓ Kronos代码导入成功")
    except ImportError as e:
        print(f"   ❌ 代码导入失败: {e}")
else:
    print("   ⚠️  etg_ai目录不存在")

# 测试预测器
try:
    sys.path.insert(0, "predictor")
    from krnos_predictor import KrnosPredictor
    print("   ✓ KrnosPredictor导入成功")
except Exception as e:
    print(f"   ⚠️  KrnosPredictor导入失败: {e}")
PYTHON

echo ""
echo "============================================================"

