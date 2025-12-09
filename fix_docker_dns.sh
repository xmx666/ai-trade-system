#!/bin/bash

echo "=== Docker DNS问题修复 ==="
echo ""
echo "问题: DNS解析失败，无法访问镜像源"
echo "解决方案: 禁用镜像源，使用VPN + 官方Docker Hub"
echo ""

# 步骤1: 禁用systemd代理配置（如果存在）
echo "步骤1: 检查systemd代理配置..."
if [ -f /etc/systemd/system/docker.service.d/http-proxy.conf ]; then
    echo "⚠️  发现systemd代理配置，保留（VPN需要代理）"
else
    echo "✅ 未找到systemd代理配置"
fi

# 步骤2: 更新镜像源配置（禁用镜像源，使用官方源）
echo ""
echo "步骤2: 更新镜像源配置（使用官方Docker Hub）..."
sudo tee /etc/docker/daemon.json > /dev/null <<'EOF'
{
  "features": {
    "buildkit": true
  },
  "registry-mirrors": [],
  "runtimes": {
    "nvidia": {
      "args": [],
      "path": "nvidia-container-runtime"
    }
  }
}
EOF
echo "✅ 镜像源配置已更新（使用官方Docker Hub）"

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

# 步骤5: 测试DNS解析
echo ""
echo "步骤5: 测试DNS解析..."
if nslookup registry-1.docker.io > /dev/null 2>&1; then
    echo "✅ DNS解析正常"
else
    echo "⚠️  DNS解析可能有问题，请检查网络连接"
fi

echo ""
echo "=== 修复完成 ==="
echo ""
echo "重要提示:"
echo "  1. 确保VPN已连接并正常工作"
echo "  2. 确保可以访问 https://hub.docker.com"
echo "  3. 如果VPN不稳定，可能需要配置DNS服务器"
echo ""
echo "现在可以重新构建："
echo "  ./start.sh start --build"
echo ""
echo "如果仍然失败，可以尝试："
echo "  1. 检查VPN连接：curl -I https://hub.docker.com"
echo "  2. 配置DNS服务器（编辑 /etc/resolv.conf）："
echo "     nameserver 8.8.8.8"
echo "     nameserver 114.114.114.114"

