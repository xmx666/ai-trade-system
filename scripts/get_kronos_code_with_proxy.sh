#!/bin/bash
# 使用代理获取Kronos代码文件

set -e

cd "$(dirname "$0")/.."

echo "============================================================"
echo "获取Kronos代码文件"
echo "============================================================"
echo ""

# 读取代理配置
PROXY_CONFIG_FILE=".env"
if [ -f "$PROXY_CONFIG_FILE" ]; then
    source <(grep -E "^HTTP_PROXY=|^HTTPS_PROXY=" "$PROXY_CONFIG_FILE" | sed 's/^/export /')
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

# 创建目录
mkdir -p ../etg_ai

# 下载代码文件
python3 << 'PYTHON'
import os
import requests
from pathlib import Path

# 配置代理
http_proxy = os.environ.get('HTTP_PROXY') or os.environ.get('http_proxy')
https_proxy = os.environ.get('HTTPS_PROXY') or os.environ.get('https_proxy')

proxies = {}
if http_proxy:
    proxies['http'] = http_proxy
if https_proxy:
    proxies['https'] = https_proxy

if proxies:
    print(f"使用代理: {proxies}")

etg_ai_dir = Path("../etg_ai")
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
        response = requests.get(url, proxies=proxies if proxies else None, timeout=30)
        if response.status_code == 200:
            file_path.write_text(response.text, encoding='utf-8')
            print(f"✓ {filename} 下载成功")
            downloaded.append(filename)
        else:
            print(f"⚠️  {filename} 无法访问 (状态码: {response.status_code})")
    except Exception as e:
        print(f"⚠️  {filename} 下载失败: {e}")

print("\n" + "=" * 60)
if len(downloaded) == len(files):
    print("✓ 所有代码文件已获取")
else:
    print(f"⚠️  部分文件需要手动获取 ({len(downloaded)}/{len(files)})")
    print("   请访问: https://huggingface.co/NeoQuasar/Kronos-base")
    print("   查看 'Files' 标签，手动下载缺失的文件")
print("=" * 60)
PYTHON

echo ""

