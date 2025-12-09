# Kronos模型设置说明

## ⚠️ 重要提示

如果遇到Microsoft Store弹窗，请在**WSL终端**中运行命令，而不是Windows PowerShell或CMD。

## 🚀 一键完成设置

在WSL终端中运行：

```bash
cd /home/xiemx/nofx
bash scripts/setup_kronos_simple_direct.sh
```

这个脚本会：
1. 自动读取 `.env` 中的代理配置
2. 下载Kronos代码文件（model.py, tokenizer.py）
3. 下载Kronos模型文件（Kronos-base, Kronos-Tokenizer-base）

## 📋 分步执行（如果一键脚本有问题）

### 步骤1: 下载代码文件

```bash
cd /home/xiemx/nofx
export HTTP_PROXY=http://172.27.128.1:7890
export HTTPS_PROXY=http://172.27.128.1:7890
python3 scripts/download_kronos_direct.py
```

### 步骤2: 检查状态

```bash
bash scripts/check_kronos_status.sh
```

### 步骤3: 验证

```bash
cd predictor
python3 -c "from krnos_predictor import KrnosPredictor; p = KrnosPredictor(); print('✓ 成功')"
```

## ✅ 所有文件已就绪

- ✅ 代码已更新完成
- ✅ 脚本已创建完成
- ✅ 代理配置已支持
- ⏳ 只需运行下载脚本即可

## 🎯 完成标志

当看到以下输出时，表示设置完成：

```
✓ model.py 下载成功
✓ tokenizer.py 下载成功
✓ Kronos-base模型下载完成
✓ Kronos-Tokenizer-base下载完成
✓ Kronos模型加载成功
```

