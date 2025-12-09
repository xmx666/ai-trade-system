# Docker 构建卡住问题排查

## 问题现象
执行 `./start.sh start --build` 时，在"重新构建镜像"步骤卡住不动。

## 可能原因

1. **首次构建很慢**（正常情况）
   - 需要下载基础镜像（golang:1.25-alpine, alpine:latest）
   - 需要编译 TA-Lib 库（可能需要 5-10 分钟）
   - 需要下载 Go 依赖包
   - 需要编译 Go 程序

2. **网络问题**
   - 下载镜像或依赖时网络超时
   - 代理配置问题

3. **资源不足**
   - CPU/内存不足导致编译缓慢

## 解决方案

### 方案1：检查构建是否真的卡住

在**另一个终端**运行以下命令查看实时构建日志：

```bash
# 如果使用 docker compose (新版本)
docker compose logs -f

# 如果使用 docker-compose (旧版本)
docker-compose logs -f
```

如果看到日志在持续输出，说明构建正在进行中，只是比较慢，请耐心等待。

### 方案2：跳过构建，使用已有镜像启动

如果之前已经构建过镜像，可以跳过构建直接启动：

```bash
# 不使用 --build 参数
./start.sh start
```

### 方案3：查看具体构建步骤

如果想看详细的构建过程，可以在另一个终端运行：

```bash
# 查看构建进度
docker compose build --progress=plain

# 或者
docker-compose build --progress=plain
```

### 方案4：清理并重新构建

如果构建确实卡住了，可以尝试清理后重新构建：

```bash
# 1. 停止当前构建（按 Ctrl+C）

# 2. 清理 Docker 缓存
docker system prune -a

# 3. 重新构建
./start.sh start --build
```

### 方案5：分步构建（调试用）

如果想看具体在哪一步卡住，可以手动构建：

```bash
# 只构建后端
docker compose build nofx --progress=plain

# 只构建前端
docker compose build nofx-frontend --progress=plain
```

### 方案6：检查网络和代理

如果网络有问题，检查代理配置：

```bash
# 检查 .env 文件中的代理设置
cat .env | grep PROXY

# 如果需要设置代理，编辑 .env 文件
# HTTP_PROXY=http://your-proxy:port
# HTTPS_PROXY=http://your-proxy:port
```

## 常见构建步骤耗时参考

- **下载基础镜像**: 1-5 分钟（取决于网络）
- **编译 TA-Lib**: 5-10 分钟（首次构建）
- **下载 Go 依赖**: 1-3 分钟
- **编译 Go 程序**: 1-3 分钟
- **构建前端**: 2-5 分钟

**总耗时**: 首次构建可能需要 10-25 分钟

## 快速启动（如果已有镜像）

如果之前已经成功构建过，可以直接启动，不需要重新构建：

```bash
./start.sh start
```

## 查看构建日志

构建过程中的日志会显示当前执行的步骤，例如：
- `Step 1/10 : FROM alpine:latest`
- `Step 2/10 : RUN apk update...`
- `Step 3/10 : RUN wget http://...`

如果某个步骤长时间没有输出，可能是：
1. 正在下载大文件（网络慢）
2. 正在编译（CPU 使用率高）
3. 真的卡住了（需要中断重试）

## 建议

1. **首次构建**: 耐心等待 15-30 分钟是正常的
2. **后续启动**: 使用 `./start.sh start`（不加 --build）会快很多
3. **查看日志**: 在另一个终端运行 `./start.sh logs` 查看实时日志
4. **资源监控**: 可以打开任务管理器查看 CPU/内存使用情况

