# Clash 代理配置指南

## 配置完成 ✅

已成功配置 Clash 代理支持，配置详情如下：

### 配置文件位置

1. **`.env` 文件** - 包含代理环境变量
2. **`docker-compose.yml`** - 已启用代理配置

### 当前配置

```bash
HTTP_PROXY=http://172.17.0.1:7890
HTTPS_PROXY=http://172.17.0.1:7890
NO_PROXY=localhost,127.0.0.1,nofx,nofx-frontend,172.21.0.0/16,172.17.0.0/16
```

### Clash 端口说明

- **HTTP 代理端口**: `7890` (默认)
- **SOCKS5 代理端口**: `7891` (如需要)
- **混合端口**: `7890` (HTTP + SOCKS5)

## 配置说明

### 代理地址选择

在 Docker 容器中访问宿主机的代理服务，需要使用特殊地址：

1. **`172.17.0.1`** - Docker 默认网桥网关（推荐，适用于大多数情况）
2. **`host.docker.internal`** - Docker Desktop 专用（如果使用 Docker Desktop）
3. **Windows 主机 IP** - 如果上述都不工作，使用 Windows 的实际 IP 地址

### 如何查找 Windows 主机 IP

在 WSL 中运行：
```bash
# 方法1：查看默认路由
ip route show | grep default

# 方法2：查看DNS服务器IP
cat /etc/resolv.conf | grep nameserver
```

### 如果 Clash 使用非标准端口

如果您的 Clash 配置了不同的端口（不是7890），请在 `.env` 文件中修改：

```bash
# 例如，如果Clash使用端口8888
HTTP_PROXY=http://172.17.0.1:8888
HTTPS_PROXY=http://172.17.0.1:8888
```

## 验证配置

### 1. 检查环境变量

```bash
docker exec nofx-trading env | grep PROXY
```

应该看到：
```
HTTP_PROXY=http://172.17.0.1:7890
HTTPS_PROXY=http://172.17.0.1:7890
NO_PROXY=localhost,127.0.0.1,nofx,nofx-frontend,172.21.0.0/16,172.17.0.0/16
```

### 2. 测试代理连接

```bash
# 测试访问外部网站（需要通过代理）
docker exec nofx-trading wget -O- --timeout=5 http://www.google.com

# 测试访问币安API（这是实际需要代理的）
docker exec nofx-trading wget -O- --timeout=5 https://fapi.binance.com/fapi/v1/time
```

### 3. 检查后端日志

```bash
docker logs nofx-trading --tail 50
```

应该不再看到网络超时错误。

## 常见问题

### Q1: 代理配置后仍然无法访问外部API

**解决方案：**
1. 确认 Clash 正在运行并监听 `7890` 端口
2. 检查 Clash 是否允许局域网连接（在 Clash 设置中启用）
3. 尝试修改 `.env` 中的代理地址为 `host.docker.internal:7890`
4. 检查防火墙是否阻止了连接

### Q2: 容器之间无法通信

**原因：** `NO_PROXY` 配置不正确

**解决方案：** 确保 `NO_PROXY` 包含所有 Docker 网络：
```bash
NO_PROXY=localhost,127.0.0.1,nofx,nofx-frontend,172.21.0.0/16,172.17.0.0/16
```

### Q3: 如何临时禁用代理

**方法1：** 注释掉 `.env` 中的代理配置
```bash
# HTTP_PROXY=http://172.17.0.1:7890
# HTTPS_PROXY=http://172.17.0.1:7890
```

**方法2：** 设置为空值
```bash
HTTP_PROXY=
HTTPS_PROXY=
```

然后重启服务：
```bash
docker compose restart
```

### Q4: Clash 运行在 Windows 上，容器无法连接

**解决方案：**
1. 在 Clash 设置中启用"允许局域网连接"
2. 检查 Windows 防火墙是否允许 Clash 接受连接
3. 尝试使用 Windows 主机的实际 IP 地址替代 `172.17.0.1`

## Clash 配置建议

在 Clash 配置文件中，建议添加以下规则以避免 Docker 网络被代理：

```yaml
rules:
  # Docker 网络直连
  - DOMAIN-SUFFIX,docker.internal,DIRECT
  - IP-CIDR,172.17.0.0/16,DIRECT
  - IP-CIDR,172.21.0.0/16,DIRECT
  # 其他规则...
```

## 重启服务

修改配置后，需要重启服务：

```bash
docker compose down
docker compose up -d
```

## 监控代理使用情况

查看后端日志，确认代理是否正常工作：

```bash
# 实时查看日志
docker logs -f nofx-trading

# 查看最近的日志
docker logs --tail 100 nofx-trading | grep -i "timeout\|error\|proxy"
```

如果看到成功的 API 请求（没有超时错误），说明代理配置成功！

