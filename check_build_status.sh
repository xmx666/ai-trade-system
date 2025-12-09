#!/bin/bash

# Docker 构建状态检查脚本
# 适用于 Windows (Git Bash / WSL) 和 Linux

echo "=== Docker 构建状态检查 ==="
echo ""

# 检测 Docker Compose 命令
if command -v docker compose &> /dev/null; then
    COMPOSE_CMD="docker compose"
    echo "✓ 使用: docker compose"
elif command -v docker-compose &> /dev/null; then
    COMPOSE_CMD="docker-compose"
    echo "✓ 使用: docker-compose"
elif command -v docker &> /dev/null; then
    # 尝试 docker compose (新版本)
    if docker compose version &> /dev/null 2>&1; then
        COMPOSE_CMD="docker compose"
        echo "✓ 使用: docker compose"
    else
        echo "✗ Docker Compose 未找到"
        echo ""
        echo "请确保："
        echo "1. Docker Desktop 已启动"
        echo "2. 在 Windows 终端（PowerShell/CMD）中运行此脚本"
        exit 1
    fi
else
    echo "✗ Docker 未找到"
    echo ""
    echo "请确保："
    echo "1. Docker Desktop 已安装并启动"
    echo "2. 在 Windows 终端（PowerShell/CMD）中运行此脚本"
    exit 1
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "1. 检查容器状态"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
$COMPOSE_CMD ps
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "2. 检查镜像状态"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
docker images | grep -E "nofx|REPOSITORY" || echo "未找到 nofx 相关镜像"
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "3. 最近 30 行构建日志"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
$COMPOSE_CMD logs --tail=30 2>&1 | tail -30
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "4. 检查构建进程"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if ps aux 2>/dev/null | grep -E "docker.*build|compose.*build" | grep -v grep; then
    echo "✓ 发现构建进程正在运行"
else
    echo "ℹ 未发现活跃的构建进程（可能已完成或未启动）"
fi
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "5. 检查端口占用"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if command -v netstat &> /dev/null; then
    netstat -ano | grep -E "1666|3001|8080" | head -5 || echo "端口未被占用"
elif command -v ss &> /dev/null; then
    ss -tlnp | grep -E "1666|3001|8080" | head -5 || echo "端口未被占用"
else
    echo "无法检查端口占用"
fi
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "建议操作"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "如果构建正在进行："
echo "  - 查看实时日志: $COMPOSE_CMD logs -f"
echo "  - 或运行: ./start.sh logs"
echo ""
echo "如果构建已完成："
echo "  - 检查服务状态: $COMPOSE_CMD ps"
echo "  - 访问前端: http://localhost:3001"
echo "  - 访问后端: http://localhost:1666"
echo ""
echo "如果构建卡住："
echo "  - 按 Ctrl+C 中断"
echo "  - 重新运行: ./start.sh start --build"
echo ""

