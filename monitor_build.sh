#!/bin/bash

# 监控 Docker 构建进度的脚本

echo "=== Docker 构建监控 ==="
echo ""
echo "提示: 构建过程可能需要 10-25 分钟，这是正常的"
echo "如果看到日志持续输出，说明构建正在进行中"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "实时构建日志（按 Ctrl+C 退出）"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
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

# 显示实时日志
$COMPOSE_CMD logs -f

