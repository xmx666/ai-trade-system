#!/bin/bash

# 快速检查构建状态的脚本
# 自动检测项目目录

echo "=== Docker 构建状态快速检查 ==="
echo ""

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "当前目录: $(pwd)"
echo ""

# 检查 docker-compose.yml 是否存在
if [ ! -f "docker-compose.yml" ]; then
    echo "错误: 未找到 docker-compose.yml"
    echo "请确保在项目根目录运行此脚本"
    exit 1
fi

echo "✓ 找到 docker-compose.yml"
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

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "1. 容器状态"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
$COMPOSE_CMD ps
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "2. 最近 30 行日志（关键信息）"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
$COMPOSE_CMD logs --tail=30 2>&1 | tail -30
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "3. 查看实时日志（按 Ctrl+C 退出）"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "运行: $COMPOSE_CMD logs -f"
echo ""

