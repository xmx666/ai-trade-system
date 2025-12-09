# Docker镜像拉取问题修复指南

## 问题描述

构建Docker镜像时遇到错误：
```
ERROR: failed to solve: nvidia/cuda:12.0.0-runtime-ubuntu22.04: failed to resolve source metadata for docker.io/nvidia/cuda:12.0.0-runtime-ubuntu22.04: unexpected status from HEAD request to https://dockerhub.azk8s.cn/v2/nvidia/cuda/manifests/12.0.0-runtime-ubuntu22.04?ns=docker.io: 403 Forbidden
```

## 问题原因

1. **镜像源配置问题**：使用了Azure的Docker Hub镜像源（dockerhub.azk8s.cn），但该镜像源可能不支持或限制访问nvidia/cuda镜像
2. **网络问题**：镜像源可能暂时不可用
3. **镜像标签问题**：指定的CUDA版本可能不存在或已更改

## 解决方案

### 方案1：修改Docker镜像源配置（推荐）

编辑Docker的daemon.json配置文件：

**Windows路径**：`C:\Users\<用户名>\.docker\daemon.json`

**Linux/WSL路径**：`/etc/docker/daemon.json`

添加或修改为：

```json
{
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com",
    "https://mirror.baidubce.com"
  ]
}
```

或者直接使用Docker Hub（如果网络允许）：

```json
{
  "registry-mirrors": []
}
```

**重启Docker服务**：
- Windows: 重启Docker Desktop
- Linux: `sudo systemctl restart docker`

### 方案2：修改Dockerfile使用其他CUDA版本

如果12.0.0版本不可用，可以尝试：

```dockerfile
# 使用其他可用的CUDA版本
FROM nvidia/cuda:11.8.0-runtime-ubuntu22.04

# 或者使用最新的稳定版本
FROM nvidia/cuda:latest-runtime-ubuntu22.04

# 或者使用devel版本（如果需要编译）
FROM nvidia/cuda:12.0.0-devel-ubuntu22.04
```

### 方案3：使用国内镜像源直接拉取

在构建前手动拉取镜像：

```bash
# 使用国内镜像源拉取
docker pull registry.cn-hangzhou.aliyuncs.com/nvidia/cuda:12.0.0-runtime-ubuntu22.04

# 然后重新标记
docker tag registry.cn-hangzhou.aliyuncs.com/nvidia/cuda:12.0.0-runtime-ubuntu22.04 nvidia/cuda:12.0.0-runtime-ubuntu22.04
```

### 方案4：修改build_and_run.sh脚本

在构建脚本中添加镜像拉取重试逻辑：

```bash
# 在构建前先尝试拉取镜像
echo "尝试拉取基础镜像..."
docker pull nvidia/cuda:12.0.0-runtime-ubuntu22.04 || {
    echo "直接拉取失败，尝试使用国内镜像源..."
    docker pull registry.cn-hangzhou.aliyuncs.com/nvidia/cuda:12.0.0-runtime-ubuntu22.04
    docker tag registry.cn-hangzhou.aliyuncs.com/nvidia/cuda:12.0.0-runtime-ubuntu22.04 nvidia/cuda:12.0.0-runtime-ubuntu22.04
}
```

### 方案5：使用CPU模式（如果不需要GPU）

如果项目可以在CPU模式下运行，可以修改Dockerfile使用普通Ubuntu镜像：

```dockerfile
# 使用普通Ubuntu镜像（CPU模式）
FROM ubuntu:22.04

# 安装必要的依赖
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    # ... 其他依赖
```

## 快速修复步骤

1. **检查Docker配置**：
   ```bash
   # Windows (PowerShell)
   cat $env:USERPROFILE\.docker\daemon.json
   
   # Linux/WSL
   cat ~/.docker/daemon.json
   ```

2. **修改镜像源配置**（如果存在）：
   - 移除或注释掉 `dockerhub.azk8s.cn`
   - 添加其他可用的镜像源

3. **重启Docker服务**

4. **重新构建**：
   ```bash
   ./build_and_run.sh
   ```

## 验证修复

测试镜像拉取：

```bash
# 测试拉取nvidia/cuda镜像
docker pull nvidia/cuda:12.0.0-runtime-ubuntu22.04

# 如果成功，应该能看到镜像下载进度
```

## 常见镜像源

- **中科大镜像**：`https://docker.mirrors.ustc.edu.cn`
- **网易镜像**：`https://hub-mirror.c.163.com`
- **百度云镜像**：`https://mirror.baidubce.com`
- **阿里云镜像**：`https://registry.cn-hangzhou.aliyuncs.com`（需要登录）

## 注意事项

1. **nvidia/cuda镜像较大**：可能需要较长时间下载
2. **网络稳定性**：确保网络连接稳定
3. **镜像版本**：确认使用的CUDA版本是否支持你的项目
4. **GPU支持**：如果使用CPU模式，确保项目可以在CPU下运行

