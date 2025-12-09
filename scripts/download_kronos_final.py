#!/usr/bin/env python3
"""下载Kronos模型和代码文件"""

import os
import sys
from pathlib import Path
from huggingface_hub import snapshot_download
import requests

def download_models():
    """下载模型文件"""
    model_dir = Path("models/kronos")
    model_dir.mkdir(parents=True, exist_ok=True)
    
    print("=" * 60)
    print("下载Kronos模型")
    print("=" * 60)
    
    # 下载模型
    if not (model_dir / "Kronos-base").exists():
        print("\n正在下载Kronos-base模型（这可能需要几分钟）...")
        try:
            snapshot_download(
                repo_id="NeoQuasar/Kronos-base",
                local_dir=str(model_dir / "Kronos-base"),
                local_dir_use_symlinks=False
            )
            print("✓ 模型下载完成")
        except Exception as e:
            print(f"❌ 模型下载失败: {e}")
            return False
    else:
        print("✓ Kronos-base模型已存在")
    
    # 下载Tokenizer
    if not (model_dir / "Kronos-Tokenizer-base").exists():
        print("\n正在下载Kronos-Tokenizer-base...")
        try:
            snapshot_download(
                repo_id="NeoQuasar/Kronos-Tokenizer-base",
                local_dir=str(model_dir / "Kronos-Tokenizer-base"),
                local_dir_use_symlinks=False
            )
            print("✓ Tokenizer下载完成")
        except Exception as e:
            print(f"❌ Tokenizer下载失败: {e}")
            return False
    else:
        print("✓ Kronos-Tokenizer-base已存在")
    
    return True

def download_code():
    """下载代码文件"""
    etg_ai_dir = Path("../etg_ai")
    etg_ai_dir.mkdir(parents=True, exist_ok=True)
    
    print("\n" + "=" * 60)
    print("获取Kronos代码文件")
    print("=" * 60)
    
    base_url = "https://huggingface.co/NeoQuasar/Kronos-base/raw/main"
    files = ["model.py", "tokenizer.py"]
    
    downloaded = []
    for filename in files:
        file_path = etg_ai_dir / filename
        if file_path.exists():
            print(f"✓ {filename} 已存在")
            downloaded.append(filename)
            continue
            
        print(f"\n正在下载: {filename}")
        try:
            url = f"{base_url}/{filename}"
            response = requests.get(url, timeout=30)
            if response.status_code == 200:
                file_path.write_text(response.text, encoding='utf-8')
                print(f"✓ {filename} 下载成功")
                downloaded.append(filename)
            else:
                print(f"⚠️  {filename} 无法访问 (状态码: {response.status_code})")
        except Exception as e:
            print(f"⚠️  {filename} 下载失败: {e}")
    
    if downloaded:
        print(f"\n✓ 成功获取 {len(downloaded)} 个代码文件")
        return True
    else:
        print("\n⚠️  无法自动下载代码文件")
        print("   请手动访问: https://huggingface.co/NeoQuasar/Kronos-base")
        print("   查看 'Files' 标签，下载 model.py 和 tokenizer.py 到 ../etg_ai/ 目录")
        return False

if __name__ == "__main__":
    print("\n开始设置Kronos模型...\n")
    
    # 下载模型
    if not download_models():
        print("\n❌ 模型下载失败，请检查网络连接")
        sys.exit(1)
    
    # 下载代码
    code_ok = download_code()
    
    print("\n" + "=" * 60)
    print("设置完成")
    print("=" * 60)
    
    if code_ok:
        print("\n✓ 所有文件已就绪，可以开始使用Kronos模型")
    else:
        print("\n⚠️  模型已下载，但代码文件需要手动获取")
        print("   请按照上面的提示手动下载代码文件")
    
    print(f"\n模型目录: {Path('models/kronos').absolute()}")
    print(f"代码目录: {Path('../etg_ai').absolute()}")

