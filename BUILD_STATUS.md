# Docker 构建状态说明

## ⚠️ 重要：构建"卡住"是正常现象

当你运行 `docker compose up -d --build` 后，终端看起来"卡住不动"是**完全正常的**！

### 为什么看起来卡住？

1. **构建过程是后台进行的**
   - Docker 在后台下载镜像、编译代码
   - 终端不会显示详细进度（除非使用特殊参数）

2. **首次构建很慢**
   - 需要下载基础镜像（1-5分钟）
   - 需要编译 TA-Lib（5-10分钟）
   - 需要下载 Go 依赖（1-3分钟）
   - 需要编译 Go 程序（1-3分钟）
   - **总计：10-25分钟是正常的**

## ✅ 如何判断构建是否正常进行？

### 方法1：在另一个终端查看日志（推荐）

打开**新的终端窗口**，运行：

```bash
cd ~/nofx
docker compose logs -f
```

如果看到日志持续输出，说明构建正在进行！

### 方法2：使用检查脚本

在另一个终端运行：

```bash
cd ~/nofx
./check_build_progress.sh
```

### 方法3：检查 Docker 进程

```bash
# 检查是否有构建进程
ps aux | grep docker

# 或检查 Docker 容器
docker ps -a
```

## 📊 正常构建日志示例

你会看到类似这样的输出：

```
Step 1/10 : FROM alpine:latest
 ---> Pulling from library/alpine
 ---> abc123def456

Step 2/10 : RUN apk update...
 ---> Running in xyz789
 ---> def456ghi789

Step 3/10 : RUN wget http://...
 ---> Downloading...
 ---> ghi789jkl012

Building...
[+] Building 45.2s (8/10)
```

## ⏱️ 构建时间参考

| 步骤 | 首次构建 | 后续构建（有缓存） |
|------|---------|-------------------|
| 下载基础镜像 | 1-5 分钟 | 几秒（使用缓存） |
| 编译 TA-Lib | 5-10 分钟 | 5-10 分钟 |
| 下载 Go 依赖 | 1-3 分钟 | 几秒（使用缓存） |
| 编译 Go 程序 | 1-3 分钟 | 1-3 分钟 |
| **总计** | **10-25 分钟** | **5-10 分钟** |

## 🚨 什么时候需要担心？

### 构建真的卡住了（需要处理）：

1. **日志超过 5 分钟没有任何输出**
2. **某个步骤重复失败**
3. **看到明显的错误信息**

### 处理方式：

```bash
# 1. 按 Ctrl+C 中断当前终端

# 2. 清理并重新构建
docker compose down
docker system prune -f
./start.sh start --build

# 3. 在另一个终端查看详细日志
docker compose build --progress=plain
```

## 💡 最佳实践

1. **启动构建后，立即打开另一个终端查看日志**
   ```bash
   docker compose logs -f
   ```

2. **首次构建请耐心等待 15-30 分钟**

3. **后续启动使用 `./start.sh start`（不加 --build）会快很多**

4. **如果确实卡住，可以中断后重试**

## 📝 快速命令参考

```bash
# 查看实时日志（推荐）
docker compose logs -f

# 查看最近50行日志
docker compose logs --tail=50

# 检查容器状态
docker compose ps

# 检查构建进度
./check_build_progress.sh

# 监控构建（实时日志）
./monitor_build.sh
```

