#!/bin/bash
# 完整设置Kronos模型

set -e

cd "$(dirname "$0")/.."

echo "============================================================"
echo "Kronos模型完整设置"
echo "============================================================"
echo ""

# 安装依赖
echo "步骤1: 安装Python依赖..."
pip3 install -q torch transformers huggingface_hub pandas numpy safetensors requests 2>&1 | grep -v "already satisfied" || true
echo "✓ 依赖安装完成"
echo ""

# 创建目录
mkdir -p models/kronos
mkdir -p ../etg_ai

# 下载模型
echo "步骤2: 下载Kronos模型..."
python3 << 'PYTHON'
import sys
from pathlib import Path
from huggingface_hub import snapshot_download

model_dir = Path("models/kronos")

# 下载模型
if not (model_dir / "Kronos-base").exists():
    print("正在下载Kronos-base模型...")
    snapshot_download("NeoQuasar/Kronos-base", local_dir=str(model_dir / "Kronos-base"), local_dir_use_symlinks=False)
    print("✓ 模型下载完成")
else:
    print("✓ 模型已存在")

# 下载Tokenizer
if not (model_dir / "Kronos-Tokenizer-base").exists():
    print("正在下载Kronos-Tokenizer-base...")
    snapshot_download("NeoQuasar/Kronos-Tokenizer-base", local_dir=str(model_dir / "Kronos-Tokenizer-base"), local_dir_use_symlinks=False)
    print("✓ Tokenizer下载完成")
else:
    print("✓ Tokenizer已存在")
PYTHON

echo ""

# 下载代码
echo "步骤3: 获取Kronos代码文件..."
python3 << 'PYTHON'
import requests
from pathlib import Path

etg_ai_dir = Path("../etg_ai")
base_url = "https://huggingface.co/NeoQuasar/Kronos-base/raw/main"
files = ["model.py", "tokenizer.py"]

downloaded = []
for f in files:
    path = etg_ai_dir / f
    if path.exists():
        print(f"✓ {f} 已存在")
        downloaded.append(f)
        continue
    
    try:
        print(f"正在下载: {f}")
        r = requests.get(f"{base_url}/{f}", timeout=30)
        if r.status_code == 200:
            path.write_text(r.text, encoding='utf-8')
            print(f"✓ {f} 下载成功")
            downloaded.append(f)
        else:
            print(f"⚠️  {f} 无法访问")
    except Exception as e:
        print(f"⚠️  {f} 下载失败: {e}")

if len(downloaded) == len(files):
    print("✓ 所有代码文件已获取")
else:
    print("⚠️  部分文件需要手动获取")
PYTHON

echo ""
echo "============================================================"
echo "设置完成"
echo "============================================================"
echo ""
echo "验证安装..."
python3 -c "
import sys
from pathlib import Path
sys.path.insert(0, str(Path('../etg_ai').absolute()))
try:
    from model import Kronos, KronosTokenizer
    print('✓ Kronos代码导入成功')
except ImportError as e:
    print(f'⚠️  代码导入失败: {e}')
    sys.exit(1)
"

echo ""
echo "✓ Kronos模型设置完成！"

