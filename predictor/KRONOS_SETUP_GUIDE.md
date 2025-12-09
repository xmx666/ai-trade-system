# Kronos模型设置指南

## 📋 概述

Kronos是一个用于金融K线预测的基础模型，来自Hugging Face: [NeoQuasar/Kronos-base](https://huggingface.co/NeoQuasar/Kronos-base)

## 🚀 快速开始

### 方法1: 使用自动化脚本（推荐）

```bash
cd nofx
bash scripts/setup_kronos_complete.sh
```

这个脚本会：
1. 安装所有必要的Python依赖
2. 克隆Kronos GitHub项目到 `../etg_ai` 目录
3. 安装Kronos项目的依赖
4. 下载Kronos模型和Tokenizer到 `models/kronos/` 目录

### 方法2: 手动设置

#### 步骤1: 安装依赖

```bash
pip install torch transformers huggingface_hub pandas numpy safetensors
```

#### 步骤2: 克隆Kronos项目

```bash
cd ..
git clone https://github.com/NeoQuasar/Kronos.git etg_ai
cd etg_ai
pip install -r requirements.txt
cd ../nofx
```

#### 步骤3: 下载模型

```bash
cd nofx
bash scripts/download_kronos_model.sh
```

或者使用Python直接下载：

```python
from huggingface_hub import snapshot_download

# 下载模型
snapshot_download(
    repo_id="NeoQuasar/Kronos-base",
    local_dir="models/kronos/Kronos-base"
)

# 下载Tokenizer
snapshot_download(
    repo_id="NeoQuasar/Kronos-Tokenizer-base",
    local_dir="models/kronos/Kronos-Tokenizer-base"
)
```

## 📁 目录结构

设置完成后，目录结构应该是：

```
nofx/
├── models/
│   └── kronos/
│       ├── Kronos-base/          # 模型文件
│       └── Kronos-Tokenizer-base/ # Tokenizer文件
├── predictor/
│   └── krnos_predictor.py        # 预测服务
└── ../etg_ai/                     # Kronos GitHub项目
    ├── model.py
    ├── tokenizer.py
    └── ...
```

## 🔧 配置说明

### 模型参数

- **模型**: `NeoQuasar/Kronos-base`
- **Tokenizer**: `NeoQuasar/Kronos-Tokenizer-base`
- **最大上下文长度**: 512步
- **推荐历史数据**: 450步
- **推荐预测步数**: 120步

### 设备选择

代码会自动选择设备：
- 如果有GPU，使用 `cuda:0`
- 否则使用 `cpu`

可以在初始化时手动指定：

```python
predictor = KrnosPredictor(device="cuda:0")
```

## ✅ 验证安装

运行测试脚本验证安装：

```bash
cd predictor
python3 test_krnos_direct.py
```

应该看到：
- ✓ Kronos模型加载成功
- ✓ 使用真实Kronos模型进行预测
- ✓ 预测结果生成成功

## ⚠️ 注意事项

1. **模型大小**: Kronos-base模型较大（约400MB），下载需要时间
2. **GPU推荐**: 虽然可以在CPU上运行，但GPU会显著加速预测
3. **内存要求**: 建议至少8GB RAM
4. **网络要求**: 首次下载需要稳定的网络连接

## 🔍 故障排除

### 问题1: 模型下载失败

**解决方案**:
- 检查网络连接
- 使用VPN（如果需要）
- 手动从Hugging Face下载

### 问题2: 无法导入Kronos模型

**解决方案**:
- 确保 `../etg_ai` 目录存在
- 确保已安装Kronos项目的依赖
- 检查Python路径设置

### 问题3: GPU不可用

**解决方案**:
- 检查CUDA安装
- 使用CPU模式（会自动降级）
- 检查PyTorch GPU支持

## 📚 参考资源

- [Kronos Hugging Face页面](https://huggingface.co/NeoQuasar/Kronos-base)
- [Kronos GitHub项目](https://github.com/NeoQuasar/Kronos)
- [Kronos论文](https://arxiv.org/abs/2508.02739)

## 🎯 下一步

设置完成后，Kronos预测服务会自动：
1. 在系统启动时加载模型
2. 每30分钟或市场变化时进行预测
3. 将预测结果提供给AI决策系统

预测结果会作为重要的参考指标（20%权重）影响交易决策。

