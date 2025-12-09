#!/bin/bash

# 检查构建进度的脚本（不阻塞）

echo "=== 构建进度检查 ==="
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

echo "1. 检查构建进程..."
if ps aux 2>/dev/null | grep -E "docker.*build|compose.*build" | grep -v grep > /dev/null; then
    echo "   ✓ 发现构建进程正在运行"
else
    echo "   ℹ 未发现构建进程（可能已完成或未启动）"
fi

echo ""
echo "2. 最近 20 行日志..."
$COMPOSE_CMD logs --tail=20 2>&1 | tail -20

echo ""
echo "3. 容器状态..."
$COMPOSE_CMD ps

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "判断标准："
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "✅ 构建正在进行（正常）："
echo "   - 日志中有 'Step X/Y' 或 'Building...'"
echo "   - 日志中有 'Downloading...' 或 'Pulling...'"
echo "   - 日志持续更新"
echo ""
echo "✅ 构建已完成："
echo "   - 看到 'Successfully built' 或 'Successfully tagged'"
echo "   - 容器状态为 'Up'"
echo ""
echo "❌ 构建卡住（需要处理）："
echo "   - 日志超过 5 分钟没有任何输出"
echo "   - 某个步骤重复失败"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "操作建议："
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "如果构建正在进行："
echo "  - 继续等待（首次构建需要 10-25 分钟）"
echo "  - 在另一个终端运行: ./monitor_build.sh 查看实时日志"
echo ""
echo "如果构建卡住："
echo "  - 按 Ctrl+C 中断当前终端"
echo "  - 运行: ./start.sh start --build"
echo ""

