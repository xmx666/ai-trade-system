# Kronos模型快速开始指南

## 🚀 快速设置

### 步骤1: 下载模型（必须）

```bash
cd nofx
bash scripts/setup_kronos_simple.sh
```

这会下载Kronos模型和Tokenizer到 `models/kronos/` 目录。

### 步骤2: 获取Kronos项目代码（必须）

Kronos需要自定义模型类才能使用。有两种方式：

#### 方式A: 从Hugging Face Spaces获取代码

1. 访问 [Kronos Hugging Face页面](https://huggingface.co/NeoQuasar/Kronos-base)
2. 点击 "Files" 标签
3. 下载 `model.py` 和 `tokenizer.py` 等文件
4. 创建 `../etg_ai` 目录并放入这些文件

#### 方式B: 查找正确的GitHub仓库

Kronos的GitHub仓库可能在不同的位置。请：
1. 查看Hugging Face页面的 "Community" 标签
2. 或搜索 "Kronos NeoQuasar" 找到正确的仓库
3. 克隆到 `../etg_ai` 目录

### 步骤3: 验证安装

```bash
cd predictor
python3 -c "
from krnos_predictor import KrnosPredictor
predictor = KrnosPredictor()
print('✓ Kronos模型加载成功')
"
```

## 📋 当前状态

- ✅ 模型下载脚本已创建
- ✅ 代码已更新支持Kronos模型
- ⏳ 需要获取Kronos项目代码（model.py等）

## 🔍 如果遇到问题

### 问题: 找不到Kronos GitHub仓库

**解决方案**:
1. 检查Hugging Face页面的文档和链接
2. 查看Hugging Face Spaces中的示例代码
3. 联系Kronos作者获取仓库地址

### 问题: 模型下载失败

**解决方案**:
- 使用VPN（如果需要）
- 检查网络连接
- 手动从Hugging Face下载

## 📚 参考

- [Kronos Hugging Face](https://huggingface.co/NeoQuasar/Kronos-base)
- [Kronos论文](https://arxiv.org/abs/2508.02739)

