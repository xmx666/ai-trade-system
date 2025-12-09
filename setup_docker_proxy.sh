#!/bin/bash

echo "=== Docker VPN代理配置脚本 ==="
echo ""
echo "配置Docker使用VPN代理: 172.27.128.1:7890"
echo ""

# 步骤1: 创建systemd代理配置目录
echo "步骤1: 创建systemd代理配置目录..."
sudo mkdir -p /etc/systemd/system/docker.service.d
echo "✅ 目录已创建"

# 步骤2: 配置Docker代理
echo ""
echo "步骤2: 配置Docker代理..."
sudo tee /etc/systemd/system/docker.service.d/http-proxy.conf > /dev/null <<'EOF'
[Service]
Environment="HTTP_PROXY=http://172.27.128.1:7890"
Environment="HTTPS_PROXY=http://172.27.128.1:7890"
Environment="NO_PROXY=localhost,127.0.0.1,docker.io,registry-1.docker.io,*.docker.io"
EOF
echo "✅ Docker代理配置已创建"

# 步骤3: 更新Docker daemon配置（禁用镜像源，使用官方源+代理）
echo ""
echo "步骤3: 更新Docker daemon配置..."
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
echo "✅ Docker daemon配置已更新"

# 步骤4: 清理BuildKit缓存
echo ""
echo "步骤4: 清理BuildKit缓存..."
docker builder prune -af > /dev/null 2>&1
echo "✅ BuildKit缓存已清理"

# 步骤5: 重启Docker服务
echo ""
echo "步骤5: 重启Docker服务..."
sudo systemctl daemon-reload
sudo systemctl restart docker
echo "✅ Docker服务已重启"
sleep 3

# 步骤6: 验证代理配置
echo ""
echo "步骤6: 验证代理配置..."
docker info 2>/dev/null | grep -i proxy | head -5 || echo "⚠️  无法验证代理配置"

echo ""
echo "=== 配置完成 ==="
echo ""
echo "Docker现在已配置使用VPN代理:"
echo "  HTTP_PROXY=http://172.27.128.1:7890"
echo "  HTTPS_PROXY=http://172.27.128.1:7890"
echo "  NO_PROXY=localhost,127.0.0.1,docker.io,registry-1.docker.io"
echo ""
echo "现在可以重新构建："
echo "  ./start.sh start --build"
echo ""
echo "如果仍然超时，请检查："
echo "  1. VPN是否正常运行"
echo "  2. 代理地址是否正确: curl -I --proxy http://172.27.128.1:7890 https://hub.docker.com"

