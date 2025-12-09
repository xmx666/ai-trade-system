#!/bin/bash

echo "=== Docker镜像源问题快速修复 ==="
echo ""

# 步骤1: 清理BuildKit缓存
echo "步骤1: 清理BuildKit缓存..."
docker builder prune -af > /dev/null 2>&1
echo "✅ BuildKit缓存已清理"

# 步骤2: 备份当前配置
if [ -f /etc/docker/daemon.json ]; then
    echo ""
    echo "步骤2: 备份当前配置..."
    sudo cp /etc/docker/daemon.json /etc/docker/daemon.json.backup.$(date +%Y%m%d_%H%M%S)
    echo "✅ 配置已备份"
fi

# 步骤3: 更新镜像源配置
echo ""
echo "步骤3: 更新镜像源配置（使用更可靠的镜像源）..."
sudo tee /etc/docker/daemon.json > /dev/null <<'EOF'
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
echo "✅ 镜像源配置已更新"

# 步骤4: 重启Docker服务
echo ""
echo "步骤4: 重启Docker服务..."
if systemctl is-active --quiet docker 2>/dev/null; then
    sudo systemctl restart docker
    echo "✅ Docker服务已重启"
    sleep 3
else
    echo "⚠️  请手动重启Docker Desktop"
fi

# 步骤5: 验证配置
echo ""
echo "步骤5: 验证配置..."
docker info 2>/dev/null | grep -A 5 "Registry Mirrors" || echo "⚠️  无法验证，请手动检查"

echo ""
echo "=== 修复完成 ==="
echo ""
echo "现在可以重新构建："
echo "  ./start.sh start --build"
echo ""
echo "如果问题仍然存在，可以尝试："
echo "  1. 临时禁用镜像源（使用VPN + 官方源）："
echo "     编辑 /etc/docker/daemon.json，将 registry-mirrors 设为 []"
echo "  2. 检查VPN连接是否正常"
echo "  3. 手动拉取镜像测试："
echo "     docker pull node:20-alpine"

