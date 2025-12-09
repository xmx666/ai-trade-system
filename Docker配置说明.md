# Docker镜像源配置说明

## 问题
Docker拉取nvidia/cuda镜像时返回403 Forbidden错误，原因是使用了Azure镜像源（dockerhub.azk8s.cn）可能不支持或限制访问。

## 解决方案

### 方法1：使用提供的脚本（推荐）

我已经创建了自动配置脚本 `fix_docker_registry.sh`，运行：

```bash
# Linux/WSL环境（需要sudo）
sudo bash fix_docker_registry.sh

# 然后重启Docker服务
sudo systemctl restart docker
# 或者重启Docker Desktop
```

### 方法2：手动配置（Windows）

1. **打开Docker Desktop**
2. **进入设置**：
   - 点击右上角设置图标（齿轮⚙️）
   - 选择 "Docker Engine"
3. **修改配置**：
   - 找到 `registry-mirrors` 配置
   - 删除或注释掉 `dockerhub.azk8s.cn`
   - 添加以下镜像源：
   ```json
   {
     "registry-mirrors": [
       "https://docker.mirrors.ustc.edu.cn",
       "https://hub-mirror.c.163.com",
       "https://mirror.baidubce.com"
     ]
   }
   ```
4. **应用并重启**：
   - 点击 "Apply & Restart"
   - 等待Docker重启完成

### 方法3：直接使用Docker Hub（如果网络允许）

如果您的网络可以直接访问Docker Hub，可以清空镜像源配置：

```json
{
  "registry-mirrors": []
}
```

### 方法4：使用命令行配置（Linux/WSL）

```bash
# 创建配置目录
sudo mkdir -p /etc/docker

# 创建配置文件
sudo tee /etc/docker/daemon.json > /dev/null <<EOF
{
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com",
    "https://mirror.baidubce.com"
  ]
}
EOF

# 重启Docker服务
sudo systemctl restart docker
```

## 验证配置

配置完成后，测试镜像拉取：

```bash
# 测试拉取nvidia/cuda镜像
docker pull nvidia/cuda:12.0.0-runtime-ubuntu22.04

# 如果成功，应该能看到下载进度
```

## 常用镜像源

- **中科大镜像**：`https://docker.mirrors.ustc.edu.cn`（推荐，速度快）
- **网易镜像**：`https://hub-mirror.c.163.com`
- **百度云镜像**：`https://mirror.baidubce.com`
- **阿里云镜像**：`https://registry.cn-hangzhou.aliyuncs.com`（需要登录）

## 注意事项

1. **配置后必须重启Docker**：修改配置后必须重启Docker服务才能生效
2. **镜像源可能不稳定**：如果某个镜像源不可用，Docker会自动尝试下一个
3. **nvidia/cuda镜像较大**：首次下载可能需要较长时间（几GB大小）
4. **网络稳定性**：确保网络连接稳定，避免下载中断

## 如果仍然失败

如果配置后仍然无法拉取，可以尝试：

1. **使用其他CUDA版本**：
   ```dockerfile
   FROM nvidia/cuda:11.8.0-runtime-ubuntu22.04
   ```

2. **手动从国内镜像源拉取**：
   ```bash
   docker pull registry.cn-hangzhou.aliyuncs.com/nvidia/cuda:12.0.0-runtime-ubuntu22.04
   docker tag registry.cn-hangzhou.aliyuncs.com/nvidia/cuda:12.0.0-runtime-ubuntu22.04 nvidia/cuda:12.0.0-runtime-ubuntu22.04
   ```

3. **使用CPU模式**（如果不需要GPU）：
   ```dockerfile
   FROM ubuntu:22.04
   ```

