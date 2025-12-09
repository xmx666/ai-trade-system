#!/bin/bash

echo "=== Docker代理问题修复 ==="
echo ""
echo "问题: Docker通过代理访问镜像源时连接被重置"
echo "解决方案: 禁用Docker的代理配置，让Docker直接使用镜像源"
echo ""

# 步骤1: 检查并备份systemd代理配置
if [ -f /etc/systemd/system/docker.service.d/http-proxy.conf ]; then
    echo "步骤1: 备份并禁用systemd代理配置..."
    sudo cp /etc/systemd/system/docker.service.d/http-proxy.conf /etc/systemd/system/docker.service.d/http-proxy.conf.backup.$(date +%Y%m%d_%H%M%S)
    sudo rm /etc/systemd/system/docker.service.d/http-proxy.conf
    echo "✅ systemd代理配置已禁用"
else
    echo "步骤1: 未找到systemd代理配置，跳过"
fi

# 步骤2: 检查并备份Docker CLI代理配置
if [ -f ~/.docker/config.json ]; then
    echo ""
    echo "步骤2: 备份Docker CLI配置..."
    cp ~/.docker/config.json ~/.docker/config.json.backup.$(date +%Y%m%d_%H%M%S)
    # 移除代理配置（如果存在）
    if grep -q "proxies" ~/.docker/config.json; then
        echo "⚠️  发现Docker CLI代理配置，请手动检查 ~/.docker/config.json"
    fi
    echo "✅ Docker CLI配置已备份"
else
    echo "步骤2: 未找到Docker CLI配置，跳过"
fi

# 步骤3: 更新镜像源配置（使用更可靠的镜像源）
echo ""
echo "步骤3: 更新镜像源配置..."
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

# 步骤4: 清理BuildKit缓存
echo ""
echo "步骤4: 清理BuildKit缓存..."
docker builder prune -af > /dev/null 2>&1
echo "✅ BuildKit缓存已清理"

# 步骤5: 重启Docker服务
echo ""
echo "步骤5: 重启Docker服务..."
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
echo ""
echo "如果问题仍然存在，可以尝试："
echo "  1. 临时禁用镜像源（使用VPN + 官方源）："
echo "     编辑 /etc/docker/daemon.json，将 registry-mirrors 设为 []"
echo "  2. 检查VPN连接是否稳定"
echo "  3. 手动测试镜像拉取："
echo "     docker pull node:20-alpine"

