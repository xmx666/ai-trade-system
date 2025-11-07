# VPN代理导致Docker容器网络问题解决方案

## 问题描述

当开启VPN代理时，可能会遇到以下问题：

1. **前端无法连接后端服务**：`connect() failed (111: Connection refused)`
2. **后端访问外部API失败**：网络超时错误
3. **WSL警告**：`检测到 localhost 代理配置，但未镜像到 WSL`

## 原因分析

VPN代理软件通常会：
- 拦截和重定向网络流量
- 影响Docker容器的网络配置
- 导致容器无法正常访问外部API（如币安API）
- 可能影响容器之间的内部通信

## 解决方案

### 方案1：配置Docker使用代理（推荐）

如果您的VPN提供了HTTP/HTTPS代理，可以配置Docker容器使用代理：

1. **设置环境变量**（在`.env`文件或`docker-compose.yml`中）：

```bash
# 设置代理地址（根据您的VPN软件调整）
HTTP_PROXY=http://127.0.0.1:7890
HTTPS_PROXY=http://127.0.0.1:7890
NO_PROXY=localhost,127.0.0.1,nofx,nofx-frontend,172.21.0.0/16
```

2. **更新docker-compose.yml**：

取消注释代理配置部分：
```yaml
environment:
  - HTTP_PROXY=${HTTP_PROXY}
  - HTTPS_PROXY=${HTTPS_PROXY}
  - NO_PROXY=localhost,127.0.0.1,nofx,nofx-frontend,172.21.0.0/16
```

3. **重启服务**：
```bash
docker compose down
docker compose up -d
```

### 方案2：在VPN中排除Docker网络（推荐）

如果VPN软件支持，将以下地址添加到排除列表：

- `172.21.0.0/16` - Docker网络
- `127.0.0.1` - 本地回环
- `localhost` - 本地主机

### 方案3：临时关闭VPN测试

如果只是想测试是否是VPN导致的问题：

1. 临时关闭VPN
2. 重启Docker服务：
```bash
docker compose restart
```
3. 观察是否恢复正常

### 方案4：使用系统代理而非VPN拦截

某些VPN软件（如Clash）提供了HTTP代理模式，可以：
1. 配置VPN软件提供HTTP代理（如 `http://127.0.0.1:7890`）
2. 使用方案1配置Docker使用该代理
3. 这样既能使用VPN，又不会影响Docker网络

## 验证修复

1. **检查容器状态**：
```bash
docker ps
```

2. **测试后端连接**：
```bash
docker exec nofx-frontend curl http://nofx:8080/api/health
```

3. **检查后端日志**：
```bash
docker logs nofx-trading --tail 50
```

应该不再看到网络超时错误。

## 常见VPN软件代理端口

- **Clash**: `http://127.0.0.1:7890`
- **V2Ray**: `http://127.0.0.1:10809`
- **Shadowsocks**: `http://127.0.0.1:1080`
- **其他**: 请查看VPN软件的设置页面

## 注意事项

1. **NO_PROXY配置很重要**：确保容器之间的通信不走代理
2. **重启服务**：修改配置后需要重启容器才能生效
3. **防火墙规则**：某些VPN会修改防火墙规则，可能阻止Docker网络

## 如果问题仍然存在

1. 检查Docker网络：
```bash
docker network inspect nofx-network
```

2. 检查容器日志：
```bash
docker logs nofx-trading
docker logs nofx-frontend
```

3. 检查DNS解析：
```bash
docker exec nofx-frontend nslookup nofx
```

4. 重启Docker服务：
```bash
sudo systemctl restart docker  # Linux
# 或重启Docker Desktop (Windows/Mac)
```

