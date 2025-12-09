#!/bin/bash

# 检查 Docker 构建状态的脚本

echo "=== 检查 Docker 构建状态 ==="
echo ""

# 检查是否有正在运行的构建进程
echo "1. 检查 Docker 构建进程..."
if command -v docker &> /dev/null; then
    docker ps -a
    echo ""
    echo "2. 检查 Docker Compose 状态..."
    if command -v docker-compose &> /dev/null; then
        docker-compose ps
    elif command -v docker &> /dev/null && docker compose version &> /dev/null; then
        docker compose ps
    fi
    echo ""
    echo "3. 查看最近的构建日志..."
    if command -v docker-compose &> /dev/null; then
        docker-compose logs --tail=100 2>&1 | tail -50
    elif command -v docker &> /dev/null && docker compose version &> /dev/null; then
        docker compose logs --tail=100 2>&1 | tail -50
    fi
else
    echo "Docker 命令不可用，可能需要在 Windows 上运行"
    echo "或者 Docker Desktop 未启动"
fi

echo ""
echo "=== 建议 ==="
echo "1. 如果构建卡住，可以尝试："
echo "   - 按 Ctrl+C 中断"
echo "   - 运行: ./start.sh start  (不使用 --build，使用已有镜像)"
echo ""
echo "2. 如果需要重新构建，可以尝试："
echo "   - 清理缓存: docker system prune -a"
echo "   - 重新构建: ./start.sh start --build"
echo ""
echo "3. 查看实时构建日志："
echo "   - 在另一个终端运行: ./start.sh logs"

