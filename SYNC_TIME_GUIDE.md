# 时间同步操作指南

## ✅ 已完成的自动化

1. ✅ 已创建时间同步脚本
2. ✅ 已重启Docker容器
3. ✅ Windows时间服务正在运行

## 🔧 需要手动完成的操作

### 方法1：通过Windows设置同步（最简单）

1. **打开Windows设置**
   - 按 `Win + I` 键
   - 或点击开始菜单 > 设置

2. **进入时间设置**
   - 点击左侧 **"时间和语言"**
   - 点击 **"日期和时间"**

3. **同步时间**
   - 确保 **"自动设置时间"** 开关已打开
   - 点击 **"立即同步"** 按钮
   - 等待同步完成（通常几秒钟）

4. **验证同步**
   ```powershell
   # 在PowerShell中运行
   Get-Date
   ```

### 方法2：使用管理员权限同步（如果方法1失败）

1. **以管理员身份运行PowerShell**
   - 右键点击开始菜单
   - 选择 **"Windows PowerShell (管理员)"**
   - 或搜索 "PowerShell"，右键选择 **"以管理员身份运行"**

2. **运行同步命令**
   ```powershell
   w32tm /resync
   ```

3. **验证同步**
   ```powershell
   w32tm /query /status
   ```

### 方法3：使用提供的脚本

1. **以管理员身份运行PowerShell**
   - 右键点击开始菜单
   - 选择 **"Windows PowerShell (管理员)"**

2. **运行同步脚本**
   ```powershell
   cd \\wsl$\Ubuntu\home\xiemx\nofx
   .\scripts\sync-time.ps1
   ```

## 🔄 重启Docker容器

时间同步后，需要重启容器以应用新时间：

```bash
# 在WSL中运行
docker compose restart nofx
```

或使用提供的脚本：
```bash
bash scripts/sync-time.sh
```

## ✅ 验证修复

### 1. 检查时间偏移

```bash
docker logs nofx-trading | grep -i "时间偏移\|Binance时间" | tail -5
```

应该看到偏移小于1000ms，例如：
```
✓ Binance时间偏移检测: -500 ms
```

### 2. 检查API调用

```bash
docker logs nofx-trading | grep -i "API调用\|获取账户\|获取持仓" | tail -10
```

不应该再看到 `-1021` 错误。

### 3. 实时监控

```bash
docker logs -f nofx-trading
```

观察是否还有时间戳相关的错误。

## 📊 当前状态

- ✅ Windows时间服务：**Running (Automatic)**
- ✅ Docker容器：**已重启**
- ⚠️  时间同步：**需要手动触发**（需要管理员权限）

## 🎯 下一步操作

1. **立即操作**：按照"方法1"在Windows设置中同步时间
2. **重启容器**：`docker compose restart nofx`
3. **验证结果**：检查日志确认时间偏移已减小

## 💡 提示

- 如果时间同步后仍然有问题，可能需要等待几分钟让时间稳定
- 某些VPN可能会影响时间同步，可以临时关闭VPN测试
- 建议定期检查时间同步状态，确保系统时间准确

## 🔍 故障排除

### 如果时间同步失败：

1. **检查网络连接**
   - 确保能访问时间服务器
   - 检查防火墙设置

2. **检查时间服务**
   ```powershell
   Get-Service -Name "W32Time"
   ```

3. **手动设置时间源**
   ```powershell
   w32tm /config /manualpeerlist:"pool.ntp.org" /syncfromflags:manual /reliable:yes /update
   w32tm /resync
   ```

### 如果容器时间仍然不准确：

1. **检查Docker配置**
   ```yaml
   volumes:
     - /etc/localtime:/etc/localtime:ro
   ```

2. **重启Docker服务**
   ```bash
   # 在WSL中
   sudo service docker restart
   ```

