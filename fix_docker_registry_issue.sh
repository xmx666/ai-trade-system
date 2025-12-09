#!/bin/bash

# Docker镜像源问题修复脚本
# 解决 "encountered unknown type text/html" 错误

echo "=== Docker镜像源问题修复 ==="
echo ""

# 备份当前配置
if [ -f /etc/docker/daemon.json ]; then
    echo "备份当前配置..."
    sudo cp /etc/docker/daemon.json /etc/docker/daemon.json.backup.$(date +%Y%m%d_%H%M%S)
fi

# 创建新的配置（使用更可靠的镜像源）
echo "创建新的Docker配置..."
sudo tee /etc/docker/daemon.json > /dev/null <<EOF
{
  "features": {
    "buildkit": true
  },
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com",
    "https://mirror.baidubce.com"
  ],
  "runtimes": {
    "nvidia": {
      "args": [],
      "path": "nvidia-container-runtime"
    }
  }
}
EOF

echo ""
echo "✅ 配置已更新"
echo ""
echo "重启Docker服务..."
sudo systemctl restart docker 2>/dev/null || echo "⚠️  请手动重启Docker Desktop"

echo ""
echo "等待Docker服务启动..."
sleep 3

echo ""
echo "验证配置..."
docker info | grep -A 10 "Registry Mirrors" || echo "⚠️  无法验证，请手动检查"

echo ""
echo "=== 修复完成 ==="
echo ""
echo "如果问题仍然存在，可以尝试："
echo "1. 临时禁用镜像源（使用官方源 + VPN）："
echo "   sudo tee /etc/docker/daemon.json > /dev/null <<'EOF'"
echo "   {"
echo "     \"features\": { \"buildkit\": true },"
echo "     \"registry-mirrors\": [],"
echo "     \"runtimes\": {"
echo "       \"nvidia\": {"
echo "         \"args\": [],"
echo "         \"path\": \"nvidia-container-runtime\""
echo "       }"
echo "     }"
echo "   }"
echo "   EOF"
echo ""
echo "2. 清理BuildKit缓存："
echo "   docker builder prune -af"
echo ""
echo "3. 重新构建："
echo "   ./start.sh start --build"

