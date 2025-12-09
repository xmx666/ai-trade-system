# 网络问题修复说明

## 问题分析

你遇到的错误：
```
WARNING: updating and opening https://dl-cdn.alpinelinux.org/alpine/v3.22/main: temporary error (try again later)
ERROR: process ... did not complete successfully: exit code: 4
```

这是典型的网络连接问题，Alpine Linux 包管理器无法连接到官方镜像源。

## 已实施的修复

### 1. **添加代理支持**
- Dockerfile 现在会从构建参数读取代理配置
- start.sh 会自动从 .env 文件读取代理配置并传递给构建过程

### 2. **使用国内镜像源**
- 自动切换到阿里云镜像源（`mirrors.aliyun.com`）
- 如果代理可用，会优先使用代理

### 3. **添加重试机制**
- 如果第一次失败，会自动重试 3 次
- 每次重试间隔 5 秒

## 现在请重新构建

### 方法1：使用优化后的脚本（推荐）

```bash
./start.sh start --build
```

脚本会自动：
- 读取 .env 中的代理配置
- 传递给 Docker 构建过程
- 使用国内镜像源作为备选

### 方法2：如果还是失败，手动指定镜像源

如果自动切换镜像源还是有问题，可以手动编辑 Dockerfile：

```dockerfile
# 在 Dockerfile.backend 中，将：
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories || true

# 改为其他镜像源，例如：
# 清华大学镜像源
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.tuna.tsinghua.edu.cn/g' /etc/apk/repositories || true

# 或者中科大镜像源
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories || true
```

## 验证代理配置

检查你的 .env 文件中的代理配置：

```bash
cat .env | grep PROXY
```

应该看到：
```
HTTP_PROXY=http://172.27.128.1:7890
HTTPS_PROXY=http://172.27.128.1:7890
```

## 如果代理不可用

如果代理不可用，可以：

1. **临时禁用代理**（编辑 .env）：
   ```bash
   # 注释掉代理配置
   # HTTP_PROXY=http://172.27.128.1:7890
   # HTTPS_PROXY=http://172.27.128.1:7890
   ```

2. **使用国内镜像源**（已自动配置）

3. **检查网络连接**：
   ```bash
   # 测试 Alpine 镜像源连接
   curl -I https://mirrors.aliyun.com/alpine/v3.22/main/x86_64/APKINDEX.tar.gz
   ```

## 常见镜像源

如果阿里云镜像源也有问题，可以尝试：

- **清华大学**: `mirrors.tuna.tsinghua.edu.cn`
- **中科大**: `mirrors.ustc.edu.cn`
- **华为云**: `mirrors.huaweicloud.com`

修改 Dockerfile 中的 `sed` 命令即可切换。

