#!/bin/bash

# 诊断构建问题的脚本

echo "=== Docker 构建诊断 ==="
echo ""

# 检测 Docker Compose 命令
if command -v docker compose &> /dev/null; then
    COMPOSE_CMD="docker compose"
elif command -v docker-compose &> /dev/null; then
    COMPOSE_CMD="docker-compose"
elif docker compose version &> /dev/null 2>&1; then
    COMPOSE_CMD="docker compose"
else
    echo "✗ Docker Compose 未找到"
    exit 1
fi

echo "1. 检查所有容器（包括已停止的）..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
$COMPOSE_CMD ps -a
echo ""

echo "2. 检查镜像..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
docker images | grep -E "nofx|REPOSITORY" || echo "未找到 nofx 相关镜像"
echo ""

echo "3. 检查构建进程..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if ps aux 2>/dev/null | grep -E "docker.*build|compose.*build" | grep -v grep; then
    echo "✓ 发现构建进程正在运行"
else
    echo "ℹ 未发现构建进程"
fi
echo ""

echo "4. 尝试获取所有日志（包括构建日志）..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
$COMPOSE_CMD logs --tail=100 2>&1 | tail -50 || echo "无法获取日志"
echo ""

echo "5. 检查 Docker 服务状态..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if docker info &> /dev/null; then
    echo "✓ Docker 服务正在运行"
else
    echo "✗ Docker 服务未运行或无法访问"
    echo "   请确保 Docker Desktop 已启动"
fi
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "诊断结果和建议"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 检查是否有容器
if $COMPOSE_CMD ps -a 2>&1 | grep -q "nofx"; then
    echo "✓ 发现容器存在"
    echo "  建议: 检查容器状态和日志"
else
    echo "ℹ 未发现容器"
    echo "  可能原因:"
    echo "    1. 构建还未开始"
    echo "    2. 构建失败"
    echo "    3. 构建正在进行中（容器还未创建）"
    echo ""
    echo "  建议操作:"
    echo "    1. 检查第一个终端是否还在运行构建命令"
    echo "    2. 如果第一个终端卡住，可以尝试重新运行:"
    echo "       ./start.sh start --build"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "下一步操作"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "如果构建还未开始："
echo "  ./start.sh start --build"
echo ""
echo "如果构建正在进行（第一个终端还在运行）："
echo "  继续等待，首次构建需要 10-25 分钟"
echo "  可以在第一个终端按 Ctrl+C 中断，然后查看日志"
echo ""
echo "如果构建已完成："
echo "  docker compose ps        # 检查容器状态"
echo "  docker compose logs       # 查看日志"
echo ""

