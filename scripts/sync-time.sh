#!/bin/bash
# WSL/容器时间同步脚本

echo "========================================="
echo "时间同步工具"
echo "========================================="
echo ""

# 显示当前时间
echo "当前WSL时间:"
date
echo ""

# 显示容器时间
echo "当前容器时间:"
docker exec nofx-trading date 2>/dev/null || echo "容器未运行"
echo ""

# 检查时间偏移（如果容器在运行）
if docker ps | grep -q nofx-trading; then
    echo "检查币安时间偏移..."
    docker logs nofx-trading 2>&1 | grep -i "时间偏移\|Binance时间" | tail -3
    echo ""
fi

echo "========================================="
echo "提示:"
echo "1. Windows时间同步需要在Windows中完成"
echo "2. WSL时间会自动跟随Windows时间"
echo "3. Docker容器时间通过 /etc/localtime 同步"
echo "========================================="

