# 检查 Docker 构建状态

## 方法1：使用检查脚本（推荐）

### Windows PowerShell
```powershell
.\check_build_status.ps1
```

### Windows CMD / Git Bash / WSL
```bash
./check_build_status.sh
```

## 方法2：手动检查

### 1. 检查容器状态
```bash
docker compose ps
# 或
docker-compose ps
```

### 2. 查看构建日志
```bash
docker compose logs --tail=50
# 或
docker-compose logs --tail=50
```

### 3. 查看实时日志（推荐）
```bash
docker compose logs -f
# 或
docker-compose logs -f
```

### 4. 检查镜像
```bash
docker images | grep nofx
```

## 方法3：使用 start.sh 脚本

```bash
# 查看日志
./start.sh logs

# 查看状态
./start.sh status
```

## 构建状态判断

### ✅ 构建正在进行
- 日志持续输出
- 看到 "Step X/Y" 或 "Building..." 等消息
- CPU/内存使用率较高

### ✅ 构建已完成
- 看到 "Successfully built" 或 "Successfully tagged"
- 容器状态为 "Up" 或 "running"
- 端口可以访问

### ❌ 构建卡住
- 日志长时间无输出（超过5分钟）
- 没有 CPU/内存活动
- 某个步骤重复失败

## 如果构建卡住

1. **中断构建**：按 `Ctrl+C`
2. **清理并重试**：
   ```bash
   docker compose down
   docker system prune -f
   ./start.sh start --build
   ```

3. **查看详细错误**：
   ```bash
   docker compose build --progress=plain
   ```

## 常见构建步骤

1. **下载基础镜像** (1-5分钟)
   - `FROM golang:1.25-alpine`
   - `FROM alpine:latest`

2. **编译 TA-Lib** (5-10分钟)
   - `RUN wget http://...ta-lib...`
   - `RUN ./configure && make && make install`

3. **下载 Go 依赖** (1-3分钟)
   - `RUN go mod download`

4. **编译 Go 程序** (1-3分钟)
   - `RUN go build -o nofx .`

5. **构建前端** (2-5分钟)
   - `RUN npm install`
   - `RUN npm run build`

**总耗时**: 首次构建 10-25 分钟是正常的

