# 构建问题排查和修复

## 问题：`docker compose logs -f` 没有输出

### 可能的原因

1. **构建还未开始**
   - 第一个终端还在运行 `docker compose up -d --build`
   - 构建命令还在执行中

2. **构建正在进行，但容器还未创建**
   - Docker 在后台构建镜像
   - 容器只有在构建完成后才会创建

3. **构建失败**
   - 构建过程中出错
   - 需要查看错误信息

4. **Docker 服务问题**
   - Docker Desktop 未启动
   - Docker 服务异常

## 解决步骤

### 步骤1：诊断问题

运行诊断脚本：

```bash
./diagnose_build.sh
```

这会检查：
- 容器状态
- 镜像状态
- 构建进程
- Docker 服务状态

### 步骤2：检查第一个终端

**重要**：检查运行 `docker compose up -d --build` 的那个终端：

- **如果还在运行**：这是正常的，构建需要时间，继续等待
- **如果已经完成**：应该看到类似 "Successfully built" 的消息
- **如果显示错误**：记录错误信息

### 步骤3：根据情况处理

#### 情况A：第一个终端还在运行（正常）

这是**正常情况**！构建需要 10-25 分钟。

**操作**：
1. 让第一个终端继续运行
2. 等待构建完成
3. 构建完成后，容器会自动启动
4. 然后可以运行 `docker compose logs -f` 查看日志

#### 情况B：第一个终端已经完成但没有容器

可能构建失败或容器启动失败。

**操作**：
```bash
# 查看构建日志
docker compose build --progress=plain

# 查看错误信息
docker compose logs

# 重新构建
./start.sh start --build
```

#### 情况C：第一个终端卡住不动

可能真的卡住了。

**操作**：
```bash
# 1. 在第一个终端按 Ctrl+C 中断

# 2. 清理并重新构建
docker compose down
docker system prune -f
./start.sh start --build

# 3. 在另一个终端查看日志
docker compose logs -f
```

## 快速修复命令

如果构建确实有问题，可以尝试：

```bash
# 1. 停止所有容器
docker compose down

# 2. 清理未使用的资源
docker system prune -f

# 3. 重新构建（显示详细进度）
docker compose build --progress=plain

# 4. 启动容器
docker compose up -d

# 5. 查看日志
docker compose logs -f
```

## 验证构建是否成功

构建成功后，应该能看到：

```bash
# 1. 容器在运行
docker compose ps
# 应该显示 nofx-trading 和 nofx-frontend 状态为 "Up"

# 2. 有日志输出
docker compose logs --tail=50
# 应该显示应用启动日志

# 3. 可以访问服务
# 前端: http://localhost:3001
# 后端: http://localhost:1666
```

## 常见错误和解决方案

### 错误1：端口被占用

```bash
# 检查端口占用
netstat -ano | grep -E "1666|3001|8080"

# 如果被占用，修改 .env 文件中的端口
```

### 错误2：构建超时

```bash
# 增加构建超时时间
export COMPOSE_HTTP_TIMEOUT=300
docker compose up -d --build
```

### 错误3：内存不足

```bash
# 清理 Docker 资源
docker system prune -a

# 重启 Docker Desktop
```

## 最佳实践

1. **首次构建**：耐心等待 15-30 分钟
2. **查看进度**：在另一个终端运行 `docker compose logs -f`
3. **如果卡住**：等待 5 分钟后，如果仍无输出，可以中断重试
4. **构建完成后**：使用 `./start.sh start`（不加 --build）会快很多

