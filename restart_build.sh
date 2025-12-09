#!/bin/bash

# 重新启动构建的脚本

echo "=== 重新启动 Docker 构建 ==="
echo ""

# 检测 Docker Compose 命令
if command -v docker compose &> /dev/null; then
    COMPOSE_CMD="docker compose"
elif command -v docker-compose &> /dev/null; then
    COMPOSE_CMD="docker-compose"
elif docker compose version &> /dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
else
    echo "错误: Docker Compose 未找到"
    exit 1
fi

echo "1. 停止现有容器（如果有）..."
$COMPOSE_CMD down 2>&1

echo ""
echo "2. 清理未使用的资源..."
docker system prune -f 2>&1 | head -5

echo ""
echo "3. 开始重新构建并启动..."
echo "   提示: 构建可能需要 10-25 分钟"
echo "   可以在另一个终端运行 './start.sh logs' 查看进度"
echo ""

$COMPOSE_CMD up -d --build --force-recreate

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "构建已启动！"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "查看实时日志:"
echo "  $COMPOSE_CMD logs -f"
echo "  或: ./start.sh logs"
echo ""
echo "检查状态:"
echo "  $COMPOSE_CMD ps"
echo "  或: ./start.sh status"
echo ""

