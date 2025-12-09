#!/bin/bash

# 应用代码修改的脚本
# 用于在 Docker 构建卡住时，提供替代方案

echo "=== 应用代码修改 ==="
echo ""

# 检测 Docker Compose 命令
if command -v docker compose &> /dev/null; then
    COMPOSE_CMD="docker compose"
elif command -v docker-compose &> /dev/null; then
    COMPOSE_CMD="docker-compose"
else
    echo "错误: Docker Compose 未安装"
    exit 1
fi

echo "方案1: 强制重新构建（推荐）"
echo "这会重新编译所有代码，应用最新修改"
echo ""
read -p "是否继续？(y/n): " choice

if [ "$choice" = "y" ] || [ "$choice" = "Y" ]; then
    echo ""
    echo "正在强制重新构建..."
    echo "提示: 构建可能需要 10-25 分钟，请耐心等待"
    echo "可以在另一个终端运行 './start.sh logs' 查看进度"
    echo ""
    
    # 停止现有容器
    $COMPOSE_CMD down
    
    # 清理旧镜像（可选，加快构建）
    read -p "是否清理旧镜像缓存？(y/n，清理会加快构建但需要重新下载): " clean_choice
    if [ "$clean_choice" = "y" ] || [ "$clean_choice" = "Y" ]; then
        echo "清理 Docker 缓存..."
        docker system prune -f
    fi
    
    # 重新构建并启动
    $COMPOSE_CMD up -d --build --force-recreate
    
    echo ""
    echo "构建已启动，查看日志: ./start.sh logs"
else
    echo ""
    echo "方案2: 检查是否有已构建的镜像"
    echo ""
    
    # 检查镜像是否存在
    if docker images | grep -q "nofx.*latest"; then
        echo "发现已有镜像，但可能不包含最新修改"
        echo ""
        echo "选项："
        echo "1. 重新构建（应用修改）: ./start.sh start --build"
        echo "2. 使用旧镜像启动（不应用修改）: ./start.sh start"
        echo ""
        read -p "选择 (1/2): " option
        
        if [ "$option" = "1" ]; then
            echo "正在重新构建..."
            $COMPOSE_CMD up -d --build --force-recreate
        else
            echo "使用旧镜像启动（注意：不会应用代码修改）"
            $COMPOSE_CMD up -d
        fi
    else
        echo "没有找到已构建的镜像，必须重新构建"
        echo "运行: ./start.sh start --build"
    fi
fi

