# Clash 代理连接问题修复指南

## 问题描述

错误信息：
```
dial tcp 172.17.0.1:7890: connect: connection refused
```

## 原因分析

Clash 运行在 **Windows 主机**上，而 Docker 容器默认使用 `172.17.0.1`（Docker 网桥网关）无法访问 Windows 主机上的服务。

## 解决方案

### ✅ 已修复

已更新 `.env` 文件，将代理地址从 `172.17.0.1:7890` 改为 `172.27.128.1:7890`（Windows 主机在 WSL 中的 IP 地址）。

### 当前配置

```bash
HTTP_PROXY=http://172.27.128.1:7890
HTTPS_PROXY=http://172.27.128.1:7890
NO_PROXY=localhost,127.0.0.1,nofx,nofx-frontend,172.21.0.0/16,172.17.0.0/16
```

## 重要检查项

### 1. Clash 必须启用"允许局域网连接"

**步骤：**
1. 打开 Clash 客户端
2. 进入 **设置** 或 **General**
3. 找到 **Allow LAN** 或 **允许局域网连接**
4. **启用** 该选项
5. 重启 Clash（如果需要）

**为什么需要这个？**
- 默认情况下，Clash 只接受来自本机的连接
- 启用后，WSL 和 Docker 容器才能访问 Clash 代理

### 2. Windows 防火墙设置

确保 Windows 防火墙允许 Clash 接受来自局域网的连接：

1. 打开 **Windows 安全中心**
2. 进入 **防火墙和网络保护**
3. 点击 **允许应用通过防火墙**
4. 找到 Clash 或添加新规则
5. 确保勾选 **专用** 和 **公用** 网络

### 3. 验证代理地址

如果 `172.27.128.1` 不工作，可以尝试以下方法：

#### 方法1：查找当前 Windows 主机 IP

在 WSL 中运行：
```bash
ip route show | grep default
```

输出示例：
```
default via 172.27.128.1 dev eth0 proto kernel
```

使用 `via` 后面的 IP 地址（如 `172.27.128.1`）

#### 方法2：使用 host.docker.internal

如果使用 Docker Desktop，可以尝试：
```bash
HTTP_PROXY=http://host.docker.internal:7890
HTTPS_PROXY=http://host.docker.internal:7890
```

#### 方法3：使用 Windows 主机名

某些情况下可以使用：
```bash
HTTP_PROXY=http://$(hostname).local:7890
```

## 测试代理连接

### 快速测试

```bash
# 测试代理是否可访问（返回400 Bad Request是正常的，说明代理在工作）
docker exec nofx-trading wget -O- --timeout=3 http://172.27.128.1:7890
```

### 完整测试

运行测试脚本：
```bash
bash scripts/test-proxy.sh
```

### 检查日志

```bash
# 查看最近的日志
docker logs nofx-trading --tail 50

# 实时查看日志
docker logs -f nofx-trading
```

如果不再看到 `connection refused` 错误，说明代理配置成功！

## 常见问题

### Q: 仍然显示 connection refused

**检查清单：**
1. ✅ Clash 是否正在运行？
2. ✅ Clash 是否启用了"允许局域网连接"？
3. ✅ Windows 防火墙是否允许 Clash？
4. ✅ 代理地址是否正确（使用 `ip route show | grep default` 查看）？
5. ✅ Clash 是否监听在 7890 端口？

**测试方法：**
```bash
# 在WSL中直接测试
curl -v http://172.27.128.1:7890
```

如果 WSL 中都无法连接，说明 Clash 配置有问题。

### Q: 代理连接成功但 API 仍然失败

**可能原因：**
1. Clash 的规则配置问题
2. 代理需要认证
3. 某些网站被阻止

**解决方法：**
1. 检查 Clash 的日志，查看连接详情
2. 在 Clash 中查看连接历史，确认请求是否到达
3. 临时切换 Clash 到全局模式测试

### Q: 如何验证代理是否真的在工作？

**方法1：查看容器内的环境变量**
```bash
docker exec nofx-trading env | grep PROXY
```

**方法2：测试访问需要代理的网站**
```bash
# 如果返回内容，说明代理工作正常
docker exec nofx-trading wget -O- https://www.google.com
```

**方法3：查看 Clash 的连接日志**
- 打开 Clash 客户端
- 查看 **Connections** 或 **连接** 标签页
- 应该能看到来自 Docker 容器的连接

## 如果问题仍然存在

### 备用方案1：在 WSL 中运行 Clash

如果 Windows 上的 Clash 无法被 Docker 访问，可以考虑在 WSL 中运行 Clash：

```bash
# 在WSL中安装Clash
wget https://github.com/Dreamacro/clash/releases/download/v1.18.0/clash-linux-amd64-v1.18.0.gz
gunzip clash-linux-amd64-v1.18.0.gz
chmod +x clash-linux-amd64-v1.18.0
sudo mv clash-linux-amd64-v1.18.0 /usr/local/bin/clash

# 运行Clash
clash
```

然后使用 `127.0.0.1:7890` 作为代理地址。

### 备用方案2：使用系统代理

如果 Docker 无法直接访问 Clash，可以：
1. 在 WSL 中配置系统代理
2. 让 Docker 使用 WSL 的系统代理

## 更新配置后的操作

修改 `.env` 文件后，必须重启服务：

```bash
docker compose down
docker compose up -d
```

## 验证成功标志

配置成功后，您应该看到：
- ✅ 不再有 `connection refused` 错误
- ✅ 币安 API 调用成功
- ✅ 日志中不再有网络超时错误
- ✅ Clash 的连接日志显示来自 Docker 容器的请求

