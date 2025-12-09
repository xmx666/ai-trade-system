#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""直接下载Kronos模型和代码（避免触发Windows Store）"""

import os
import sys
from pathlib import Path

# 设置代理（从.env读取）
env_file = Path(".env")
if env_file.exists():
    with open(env_file) as f:
        for line in f:
            line = line.strip()
            if line.startswith("HTTP_PROXY="):
                proxy = line.split("=", 1)[1].strip().strip('"').strip("'")
                os.environ["HTTP_PROXY"] = proxy
                os.environ["http_proxy"] = proxy
            elif line.startswith("HTTPS_PROXY="):
                proxy = line.split("=", 1)[1].strip().strip('"').strip("'")
                os.environ["HTTPS_PROXY"] = proxy
                os.environ["https_proxy"] = proxy

print("=" * 60)
print("Kronos模型和代码下载")
print("=" * 60)
print(f"代理: HTTP={os.environ.get('HTTP_PROXY')}, HTTPS={os.environ.get('HTTPS_PROXY')}")
print()

# 1. 下载代码文件
print("步骤1: 下载代码文件...")
import requests

etg_ai_dir = Path("../etg_ai")
etg_ai_dir.mkdir(parents=True, exist_ok=True)

proxies = {}
if os.environ.get("HTTP_PROXY"):
    proxies["http"] = os.environ["HTTP_PROXY"]
if os.environ.get("HTTPS_PROXY"):
    proxies["https"] = os.environ["HTTPS_PROXY"]

base_url = "https://huggingface.co/NeoQuasar/Kronos-base/raw/main"
files = ["model.py", "tokenizer.py"]

for filename in files:
    file_path = etg_ai_dir / filename
    if file_path.exists():
        print(f"  ✓ {filename} 已存在")
        continue
    
    try:
        print(f"  下载: {filename}")
        url = f"{base_url}/{filename}"
        response = requests.get(url, proxies=proxies, timeout=60)
        if response.status_code == 200:
            file_path.write_text(response.text, encoding='utf-8')
            print(f"  ✓ {filename} 下载成功")
        else:
            print(f"  ⚠️  {filename} 状态码: {response.status_code}")
    except Exception as e:
        print(f"  ❌ {filename} 失败: {e}")

print()

# 2. 下载模型
print("步骤2: 下载模型文件...")
try:
    from huggingface_hub import snapshot_download
    
    model_dir = Path("models/kronos")
    model_dir.mkdir(parents=True, exist_ok=True)
    
    # 下载模型
    if not (model_dir / "Kronos-base").exists():
        print("  正在下载Kronos-base模型（这可能需要几分钟）...")
        snapshot_download(
            "NeoQuasar/Kronos-base",
            local_dir=str(model_dir / "Kronos-base"),
            local_dir_use_symlinks=False
        )
        print("  ✓ Kronos-base模型下载完成")
    else:
        print("  ✓ Kronos-base模型已存在")
    
    # 下载Tokenizer
    if not (model_dir / "Kronos-Tokenizer-base").exists():
        print("  正在下载Kronos-Tokenizer-base...")
        snapshot_download(
            "NeoQuasar/Kronos-Tokenizer-base",
            local_dir=str(model_dir / "Kronos-Tokenizer-base"),
            local_dir_use_symlinks=False
        )
        print("  ✓ Kronos-Tokenizer-base下载完成")
    else:
        print("  ✓ Kronos-Tokenizer-base已存在")
        
except ImportError:
    print("  ❌ huggingface_hub未安装，请运行: pip install huggingface_hub")
    sys.exit(1)
except Exception as e:
    print(f"  ❌ 模型下载失败: {e}")
    import traceback
    traceback.print_exc()
    sys.exit(1)

print()
print("=" * 60)
print("✓ 所有文件下载完成")
print("=" * 60)
print(f"模型目录: {Path('models/kronos').absolute()}")
print(f"代码目录: {Path('../etg_ai').absolute()}")

