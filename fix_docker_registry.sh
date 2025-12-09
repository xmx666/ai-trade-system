#!/bin/bash

# Docker镜像源配置修复脚本

echo "=========================================="
echo "Docker镜像源配置修复"
echo "=========================================="

# 检测操作系统
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
    # Linux/WSL环境
    DOCKER_CONFIG_DIR="/etc/docker"
    DOCKER_CONFIG_FILE="$DOCKER_CONFIG_DIR/daemon.json"
    
    echo "[1/4] 检测到Linux/WSL环境"
    
    # 检查是否有sudo权限
    if [ "$EUID" -ne 0 ]; then 
        echo "⚠️  需要sudo权限来修改Docker配置"
        echo "请使用: sudo bash fix_docker_registry.sh"
        exit 1
    fi
    
    # 创建配置目录（如果不存在）
    if [ ! -d "$DOCKER_CONFIG_DIR" ]; then
        echo "[2/4] 创建Docker配置目录..."
        mkdir -p "$DOCKER_CONFIG_DIR"
    else
        echo "[2/4] Docker配置目录已存在"
    fi
    
    # 备份现有配置
    if [ -f "$DOCKER_CONFIG_FILE" ]; then
        echo "[3/4] 备份现有配置..."
        cp "$DOCKER_CONFIG_FILE" "$DOCKER_CONFIG_FILE.backup.$(date +%Y%m%d_%H%M%S)"
    fi
    
    # 创建新的配置
    echo "[4/4] 创建新的镜像源配置..."
    cat > "$DOCKER_CONFIG_FILE" << 'EOF'
{
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com",
    "https://mirror.baidubce.com"
  ],
  "insecure-registries": [],
  "debug": false,
  "experimental": false
}
EOF
    
    echo "✅ 配置已创建: $DOCKER_CONFIG_FILE"
    echo ""
    echo "配置内容:"
    cat "$DOCKER_CONFIG_FILE"
    echo ""
    echo "⚠️  请重启Docker服务以使配置生效:"
    echo "   sudo systemctl restart docker"
    echo "   或者重启Docker Desktop"
    
elif [[ "$OSTYPE" == "msys" || "$OSTYPE" == "win32" ]]; then
    # Windows环境
    echo "[1/4] 检测到Windows环境"
    echo ""
    echo "Windows环境下，请手动配置Docker Desktop:"
    echo ""
    echo "步骤："
    echo "1. 打开Docker Desktop"
    echo "2. 点击右上角设置图标（齿轮）"
    echo "3. 进入 'Docker Engine' 设置"
    echo "4. 修改或添加以下配置："
    echo ""
    cat << 'EOF'
{
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com",
    "https://mirror.baidubce.com"
  ]
}
EOF
    echo ""
    echo "5. 点击 'Apply & Restart'"
    echo ""
    echo "或者，我可以为您创建配置文件..."
    
    # Windows用户目录下的Docker配置
    USER_PROFILE=$(cmd.exe /c "echo %USERPROFILE%" 2>/dev/null | tr -d '\r')
    if [ -n "$USER_PROFILE" ]; then
        DOCKER_CONFIG_DIR="$USER_PROFILE/.docker"
        DOCKER_CONFIG_FILE="$DOCKER_CONFIG_DIR/daemon.json"
        
        echo ""
        echo "创建配置文件: $DOCKER_CONFIG_FILE"
        mkdir -p "$DOCKER_CONFIG_DIR"
        
        cat > "$DOCKER_CONFIG_FILE" << 'EOF'
{
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com",
    "https://mirror.baidubce.com"
  ]
}
EOF
        echo "✅ 配置文件已创建"
        echo "⚠️  请重启Docker Desktop以使配置生效"
    fi
else
    echo "未识别的操作系统: $OSTYPE"
    exit 1
fi

echo ""
echo "=========================================="
echo "配置完成！"
echo "=========================================="

