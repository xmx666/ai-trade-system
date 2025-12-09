# Docker 镜像源连接问题修复指南

## 问题描述
Docker 构建时无法连接到 `docker.m.daocloud.io`，出现 DNS 解析超时错误。

## 解决方案

### 方案1：修改 Docker Desktop 镜像源配置（推荐）

1. **打开 Docker Desktop**
2. **进入设置**：点击右上角设置图标 → Settings
3. **找到 Docker Engine**：左侧菜单选择 "Docker Engine"
4. **修改镜像源配置**，将以下内容添加到 JSON 配置中：

```json
{
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com",
    "https://mirror.baidubce.com"
  ]
}
```

或者使用官方源（如果网络允许）：
```json
{
  "registry-mirrors": []
}
```

5. **点击 "Apply & Restart"** 应用并重启 Docker

### 方案2：使用环境变量临时覆盖

在构建前设置环境变量，强制使用官方源：

**Windows PowerShell:**
```powershell
$env:DOCKER_BUILDKIT=0
docker-compose build --no-cache
```

**Linux/WSL:**
```bash
export DOCKER_BUILDKIT=0
docker-compose build --no-cache
```

### 方案3：检查网络连接

1. **测试 DNS 解析**：
   ```bash
   nslookup docker.m.daocloud.io
   ```

2. **测试连接**：
   ```bash
   curl -I https://docker.m.daocloud.io
   ```

3. **如果无法连接，考虑**：
   - 检查防火墙设置
   - 检查代理设置
   - 尝试使用 VPN 或更换网络

### 方案4：使用代理（如果已配置 VPN）

如果已配置代理，确保 Docker Desktop 使用系统代理：

1. **Docker Desktop Settings** → **Resources** → **Proxies**
2. **启用 "Manual proxy configuration"**
3. **填写代理地址和端口**

### 方案5：直接使用官方 Docker Hub（如果网络允许）

修改 Docker Desktop 配置，移除所有镜像源：

```json
{
  "registry-mirrors": []
}
```

然后重启 Docker Desktop。

## 验证修复

修复后，尝试重新构建：

```bash
docker-compose build
```

如果仍然失败，可以尝试：

```bash
docker-compose build --no-cache --pull
```

## 常见镜像源列表

- **中科大镜像**：`https://docker.mirrors.ustc.edu.cn`
- **网易镜像**：`https://hub-mirror.c.163.com`
- **百度云镜像**：`https://mirror.baidubce.com`
- **阿里云镜像**：需要登录阿里云获取专属地址
- **DaoCloud**：`https://docker.m.daocloud.io`（当前不可用）

## 注意事项

- 修改 Docker Desktop 配置后需要重启 Docker
- 某些镜像源可能需要登录或限制访问
- 如果使用企业网络，可能需要配置代理

