# 严格模型政策 - 禁止模拟预测

## 🚫 核心原则

**禁止使用任何模拟或备用预测方案**

为了确保交易系统的准确性和可靠性，krnos预测服务采用严格的政策：

### 1. 必须使用真实的krnos模型
- ✅ 只有etg_ai项目的krnos模型可用时才进行预测
- ❌ 禁止使用任何统计方法、趋势外推或其他模拟算法
- ❌ 禁止使用任何备用或降级预测方案

### 2. 模型不可用时禁止预测
- ❌ 如果模型未加载，直接抛出 `RuntimeError`
- ❌ 如果模型调用失败，直接抛出 `RuntimeError`
- ❌ 不返回任何预测结果（包括错误状态的结果）
- ❌ 避免误导交易系统做出错误决策

### 3. 错误处理
- 所有预测失败都抛出 `RuntimeError` 或 `NotImplementedError`
- 调用方必须处理这些错误
- 系统应该在没有预测结果的情况下继续运行（不使用预测数据）

## 📋 实现细节

### 模型加载检查

```python
def _load_model(self):
    """加载krnos模型（必须成功，否则禁止预测）"""
    if not etg_ai_path.exists():
        logger.error("etg_ai项目未找到，预测功能已禁用")
        self.model = None
        return
    
    # 如果模型加载失败
    self.model = None  # 标记为不可用
```

### 预测前检查

```python
def predict_with_monte_carlo(...):
    # 严格检查：模型必须可用
    if self.model is None:
        raise RuntimeError("krnos模型未加载，禁止进行预测")
    
    # 调用真实模型
    base_prediction = self._call_real_krnos_model(...)
```

### 模型调用

```python
def _call_real_krnos_model(...):
    """调用真实的krnos模型进行预测"""
    if self.model is None:
        raise RuntimeError("krnos模型未加载，无法进行预测")
    
    # 临时：抛出错误，禁止使用模拟预测
    raise NotImplementedError(
        "krnos模型调用功能需要根据etg_ai项目实际结构实现。"
        "当前禁止使用任何模拟预测以避免误导交易系统。"
    )
```

### 错误处理

```python
except Exception as e:
    # 不返回任何预测结果
    raise RuntimeError(f"预测失败: {e}，禁止返回任何预测结果")
```

## ⚠️ 重要说明

### 当前状态
- ❌ 模型加载功能需要根据etg_ai项目实际结构实现
- ❌ 模型调用功能需要根据etg_ai项目实际结构实现
- ✅ 当前会抛出 `NotImplementedError`，禁止任何预测
- ✅ 所有模拟预测函数已删除

### 已删除的功能
- ❌ `_simulate_prediction_simple()` - 已删除
- ❌ `_simulate_prediction_with_real_data()` - 已删除
- ❌ `_calculate_ema()` - 已删除（仅用于模拟）
- ❌ `_calculate_real_volatility()` - 已删除（仅用于模拟）
- ❌ `_smooth_prediction()` - 已删除（仅用于模拟）

### 保留的功能
- ✅ `_monte_carlo_simulation()` - 保留（用于对真实模型预测结果进行蒙特卡罗模拟）
- ✅ `_calculate_trend_strength()` - 保留（用于计算趋势强度）
- ✅ `_save_prediction()` - 保留（用于保存预测结果）

## 🔧 如何启用预测

### 步骤1: 设置etg_ai项目
```bash
bash scripts/setup_etg_ai.sh
```

### 步骤2: 实现模型加载
在 `_load_model()` 中实现真实的模型加载：
```python
def _load_model(self):
    try:
        # 根据etg_ai项目的实际结构加载模型
        from etg_ai import load_krnos_model
        self.model = load_krnos_model(self.model_path)
        logger.info("krnos模型加载成功")
    except Exception as e:
        logger.error(f"模型加载失败: {e}")
        self.model = None
```

### 步骤3: 实现模型调用
在 `_call_real_krnos_model()` 中实现真实的模型预测调用：
```python
def _call_real_krnos_model(self, price_history, volume_history, prediction_horizon):
    if self.model is None:
        raise RuntimeError("krnos模型未加载，无法进行预测")
    
    # 调用真实模型
    prediction = self.model.predict(
        price_history=np.array(price_history),
        volume_history=np.array(volume_history),
        prediction_horizon=prediction_horizon
    )
    return prediction
```

### 步骤4: 测试验证
- 确保模型可以正常加载（`self.model` 不为 `None`）
- 确保模型可以正常调用（`_call_real_krnos_model()` 实现）
- 验证预测结果的合理性

## ✅ 验证清单

- [ ] etg_ai项目已设置
- [ ] 模型可以正常加载（`self.model` 不为 `None`）
- [ ] 模型可以正常调用（`_call_real_krnos_model()` 实现）
- [ ] 预测结果格式正确
- [ ] 错误处理正确（模型不可用时抛出错误）
- [ ] 所有模拟预测函数已删除
- [ ] 没有备用预测方案

## 🎯 目标

确保交易系统只使用真实的、经过验证的模型预测结果，避免任何模拟或备用方案导致的错误决策。

## 📝 注意事项

1. **蒙特卡罗模拟**：`_monte_carlo_simulation()` 函数保留，但仅用于对真实模型预测结果进行蒙特卡罗模拟，不是替代模型预测。

2. **错误处理**：调用方必须正确处理 `RuntimeError` 和 `NotImplementedError`，不要尝试使用备用方案。

3. **系统集成**：在 `decision/engine.go` 中，如果预测失败，应该继续运行但不使用预测数据。

