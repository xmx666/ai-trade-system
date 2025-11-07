# 币安API时间戳错误修复指南

## 错误信息

```
code=-1021, msg=Timestamp for this request was 1000ms ahead of the server's time.
```

## 问题原因

本地系统时间比币安服务器时间快约1-1.5秒，导致API请求被拒绝。

## 解决方案

### 方案1：同步Windows系统时间（推荐）

1. **打开Windows设置**
   - 按 `Win + I` 打开设置
   - 进入 **时间和语言** > **日期和时间**

2. **启用自动时间同步**
   - 确保 **自动设置时间** 已启用
   - 点击 **立即同步** 按钮

3. **验证时间同步**
   ```bash
   # 在WSL中检查时间
   date
   ```

4. **重启Docker容器**
   ```bash
   docker compose restart nofx
   ```

### 方案2：手动同步时间（如果自动同步失败）

**在Windows中：**
1. 右键点击任务栏的时间
2. 选择 **调整日期/时间**
3. 点击 **立即同步**

**在WSL中：**
```bash
# 使用sudo同步时间（需要管理员权限）
sudo ntpdate -s pool.ntp.org
# 或者
sudo ntpdate -s time.nist.gov
```

### 方案3：检查Docker容器时间同步

确保 `docker-compose.yml` 中已配置时间同步：

```yaml
volumes:
  - /etc/localtime:/etc/localtime:ro  # 同步宿主机时间
environment:
  - TZ=${NOFX_TIMEZONE:-Asia/Shanghai}
```

### 方案4：验证时间偏移

查看日志中的时间偏移检测：

```bash
docker logs nofx-trading | grep "时间偏移"
```

应该看到类似：
```
✓ Binance时间偏移检测: -1200 ms
```

如果偏移超过1000ms，需要同步时间。

## 预防措施

### 1. 定期检查时间同步

```bash
# 检查系统时间同步状态（Linux）
timedatectl status

# 检查Docker容器时间
docker exec nofx-trading date
```

### 2. 确保NTP服务运行

**Windows：**
- Windows Time服务应该自动运行
- 检查服务状态：`services.msc` > 查找 "Windows Time"

**Linux/WSL：**
```bash
# 检查NTP服务状态
sudo systemctl status ntp
# 或
sudo systemctl status systemd-timesyncd
```

### 3. 如果使用VPN

某些VPN可能会影响时间同步。如果遇到问题：
- 临时关闭VPN测试
- 或使用VPN的直连模式

## 常见问题

### Q: 为什么会有时间偏移？

**原因：**
1. 系统时钟不准确（硬件时钟漂移）
2. 网络延迟导致同步失败
3. 时区设置错误
4. Docker容器时间不同步

### Q: 时间偏移检测显示-1200ms，但API仍然失败？

**说明：**
- `go-binance` 库不支持直接设置时间偏移
- 必须同步系统时间才能解决问题
- 检测到的偏移仅用于日志记录和诊断

### Q: 如何验证时间已同步？

```bash
# 1. 检查容器时间
docker exec nofx-trading date

# 2. 查看日志中的时间偏移
docker logs nofx-trading | grep "时间偏移"

# 3. 如果偏移小于500ms，说明已同步
```

### Q: 同步后仍然有问题？

**尝试：**
1. 重启Docker服务
2. 重启Windows系统
3. 检查时区设置是否正确
4. 查看是否有其他程序修改了系统时间

## 验证修复

修复后，查看日志应该：
1. ✅ 时间偏移检测显示小于1000ms
2. ✅ 不再出现 `-1021` 错误
3. ✅ API调用成功

## 技术细节

- 币安API要求时间戳与服务器时间差在 **1000ms以内**
- `go-binance` 库会自动使用本地时间生成时间戳
- Docker容器时间通过 `/etc/localtime` 挂载与宿主机同步
- 最佳实践是确保宿主机时间准确

