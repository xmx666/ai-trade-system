#!/bin/bash
# 设置etg_ai项目并下载krnos模型

set -e

ETG_AI_DIR="../etg_ai"
MODEL_DIR="./models/krnos"

echo "正在设置etg_ai项目..."

# 检查是否已存在etg_ai目录
if [ ! -d "$ETG_AI_DIR" ]; then
    echo "正在从GitHub克隆etg_ai项目..."
    cd ..
    git clone https://github.com/your-username/etg_ai.git || {
        echo "错误: 无法克隆etg_ai项目"
        echo "请确保:"
        echo "1. GitHub仓库URL正确"
        echo "2. 您有访问权限"
        echo "3. 或者手动克隆项目到 ../etg_ai 目录"
        exit 1
    }
    cd nofx
fi

# 创建模型目录
mkdir -p "$MODEL_DIR"

# 检查etg_ai项目是否存在requirements.txt
if [ -f "$ETG_AI_DIR/requirements.txt" ]; then
    echo "正在安装etg_ai项目依赖..."
    cd "$ETG_AI_DIR"
    pip install -r requirements.txt
    cd - > /dev/null
fi

# 检查是否需要下载模型
if [ ! -f "$MODEL_DIR/krnos_model.pt" ] && [ ! -f "$MODEL_DIR/krnos_model.pth" ]; then
    echo "正在下载krnos模型..."
    # 这里需要根据etg_ai项目的实际模型下载方式调整
    # 可能需要从HuggingFace或其他模型仓库下载
    echo "请参考etg_ai项目的README，手动下载模型到 $MODEL_DIR 目录"
fi

echo "etg_ai项目设置完成！"
echo "模型目录: $MODEL_DIR"
echo "项目目录: $ETG_AI_DIR"

