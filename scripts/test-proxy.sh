#!/bin/bash
# 测试Clash代理连接的脚本

echo "🔍 测试Clash代理连接..."
echo ""

# 获取Windows主机IP（WSL网关）
WINDOWS_HOST=$(ip route show | grep default | awk '{print $3}')
echo "📡 Windows主机IP: $WINDOWS_HOST"
echo ""

# 测试1: 在WSL中测试
echo "测试1: 在WSL中测试代理连接..."
if curl -s --connect-timeout 2 http://${WINDOWS_HOST}:7890 > /dev/null 2>&1; then
    echo "✅ WSL可以访问Clash代理"
else
    echo "❌ WSL无法访问Clash代理"
    echo "   请检查："
    echo "   1. Clash是否正在运行"
    echo "   2. Clash是否启用了'允许局域网连接'"
    echo "   3. Windows防火墙是否允许Clash接受连接"
fi
echo ""

# 测试2: 在Docker容器中测试
echo "测试2: 在Docker容器中测试代理连接..."
if docker exec nofx-trading wget -O- --timeout=3 http://${WINDOWS_HOST}:7890 > /dev/null 2>&1; then
    echo "✅ Docker容器可以访问Clash代理"
else
    echo "❌ Docker容器无法访问Clash代理"
    echo "   请检查.env文件中的代理配置是否正确"
fi
echo ""

# 测试3: 测试实际API访问
echo "测试3: 通过代理访问币安API..."
if docker exec nofx-trading wget -O- --timeout=5 https://fapi.binance.com/fapi/v1/time > /dev/null 2>&1; then
    echo "✅ 可以通过代理访问币安API"
else
    echo "❌ 无法通过代理访问币安API"
fi
echo ""

# 显示当前配置
echo "📋 当前代理配置:"
docker exec nofx-trading env | grep PROXY
echo ""

# 建议
echo "💡 如果代理无法连接，请尝试："
echo "   1. 在Clash中启用'允许局域网连接'"
echo "   2. 检查Windows防火墙设置"
echo "   3. 尝试使用 host.docker.internal:7890"
echo "   4. 检查Clash是否监听在7890端口"

