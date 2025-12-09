#!/bin/bash
# 最简单的Kronos设置脚本

cd "$(dirname "$0")/.."

# 读取代理
source .env 2>/dev/null || true
export HTTP_PROXY=${HTTP_PROXY:-http://172.27.128.1:7890}
export HTTPS_PROXY=${HTTPS_PROXY:-http://172.27.128.1:7890}
export http_proxy=$HTTP_PROXY
export https_proxy=$HTTPS_PROXY

echo "开始下载..."

# 直接运行Python脚本
python3 << 'END_PYTHON'
import os
import sys
import requests
from pathlib import Path
from huggingface_hub import snapshot_download

# 1. 下载代码
print("下载代码文件...")
etg_ai = Path("../etg_ai")
etg_ai.mkdir(exist_ok=True)

proxies = {"http": os.environ.get("HTTP_PROXY"), "https": os.environ.get("HTTPS_PROXY")}
for f in ["model.py", "tokenizer.py"]:
    if (etg_ai / f).exists():
        print(f"✓ {f} 已存在")
        continue
    try:
        r = requests.get(f"https://huggingface.co/NeoQuasar/Kronos-base/raw/main/{f}", proxies=proxies, timeout=60)
        if r.status_code == 200:
            (etg_ai / f).write_text(r.text)
            print(f"✓ {f} 下载成功")
    except Exception as e:
        print(f"❌ {f} 失败: {e}")

# 2. 下载模型
print("\n下载模型文件...")
model_dir = Path("models/kronos")
model_dir.mkdir(parents=True, exist_ok=True)

for repo in ["NeoQuasar/Kronos-base", "NeoQuasar/Kronos-Tokenizer-base"]:
    name = repo.split("/")[1]
    if (model_dir / name).exists():
        print(f"✓ {name} 已存在")
        continue
    try:
        print(f"下载 {name}...")
        snapshot_download(repo, local_dir=str(model_dir / name), local_dir_use_symlinks=False)
        print(f"✓ {name} 下载完成")
    except Exception as e:
        print(f"❌ {name} 失败: {e}")

print("\n✓ 完成")
END_PYTHON

