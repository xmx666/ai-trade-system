# Kronos模型设置（使用代理）

## ✅ 已创建支持代理的脚本

由于网络环境需要代理，已创建以下脚本：

### 1. 下载模型（使用代理）
```bash
cd nofx
bash scripts/download_kronos_with_proxy.sh
```

这个脚本会：
- 自动从 `.env` 文件读取代理配置
- 设置环境变量让 `huggingface_hub` 使用代理
- 下载 Kronos-base 模型和 Tokenizer

### 2. 获取代码文件（使用代理）
```bash
cd nofx
bash scripts/get_kronos_code_with_proxy.sh
```

这个脚本会：
- 使用代理从 Hugging Face 下载 `model.py` 和 `tokenizer.py`
- 保存到 `../etg_ai/` 目录

### 3. 完整设置（一键完成）
```bash
cd nofx
bash scripts/setup_kronos_complete_with_proxy.sh
```

这会依次执行：
1. 下载模型
2. 获取代码
3. 验证安装

## 📋 当前代理配置

根据 `.env` 文件：
- HTTP_PROXY: http://172.27.128.1:7890
- HTTPS_PROXY: http://172.27.128.1:7890

## ⏳ 下载进度

模型下载可能需要几分钟（Kronos-base约400MB）。

检查进度：
```bash
# 检查模型文件
ls -lh models/kronos/

# 检查代码文件
ls -la ../etg_ai/*.py

# 检查状态
bash scripts/check_kronos_status.sh
```

## ✅ 验证安装

下载完成后，运行：

```bash
cd predictor
python3 -c "
from krnos_predictor import KrnosPredictor
predictor = KrnosPredictor()
print('✓ Kronos模型加载成功')
"
```

## 🔧 如果遇到问题

### 问题1: 代理连接失败
- 确保 Clash 已启用"允许局域网连接"
- 检查代理地址是否正确
- 测试代理：`curl -x http://172.27.128.1:7890 https://huggingface.co`

### 问题2: 下载中断
- 重新运行下载脚本（会跳过已下载的文件）
- 检查网络连接
- 确保 VPN 正常工作

### 问题3: 代码文件下载失败
- 可以手动访问 https://huggingface.co/NeoQuasar/Kronos-base
- 查看 "Files" 标签，下载 `model.py` 和 `tokenizer.py`
- 保存到 `../etg_ai/` 目录

