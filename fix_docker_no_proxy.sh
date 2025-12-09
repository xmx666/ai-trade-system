#!/bin/bash

echo "=== Docker代理问题修复（禁用代理） ==="
echo ""
echo "问题: Docker通过代理访问镜像源时连接被重置"
echo "解决方案: 禁用Docker的代理配置，让Docker直接使用镜像源"
echo ""

# 步骤1: 禁用systemd代理配置
echo "步骤1: 禁用systemd代理配置..."
if [ -f /etc/systemd/system/docker.service.d/http-proxy.conf ]; then
    sudo cp /etc/systemd/system/docker.service.d/http-proxy.conf /etc/systemd/system/docker.service.d/http-proxy.conf.backup.$(date +%Y%m%d_%H%M%S)
    sudo rm /etc/systemd/system/docker.service.d/http-proxy.conf
    echo "✅ systemd代理配置已禁用"
else
    echo "✅ 未找到systemd代理配置"
fi

# 步骤2: 更新镜像源配置（不使用代理）
echo ""
echo "步骤2: 更新镜像源配置..."
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

# 步骤3: 清理BuildKit缓存
echo ""
echo "步骤3: 清理BuildKit缓存..."
docker builder prune -af > /dev/null 2>&1
echo "✅ BuildKit缓存已清理"

# 步骤4: 重启Docker服务
echo ""
echo "步骤4: 重启Docker服务..."
if systemctl is-active --quiet docker 2>/dev/null; then
    sudo systemctl daemon-reload
    sudo systemctl restart docker
    echo "✅ Docker服务已重启"
    sleep 3
else
    echo "⚠️  请手动重启Docker Desktop"
fi

echo ""
echo "=== 修复完成 ==="
echo ""
echo "现在可以重新构建："
echo "  ./start.sh start --build"

