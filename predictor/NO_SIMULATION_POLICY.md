# 禁止模拟预测政策

## 🚫 严格政策

**禁止使用任何模拟或备用预测方案**

为了确保交易系统的准确性，krnos预测服务采用严格的政策：

1. **必须使用真实的krnos模型**
   - 只有etg_ai项目的krnos模型可用时才进行预测
   - 禁止使用任何统计方法、趋势外推或其他模拟算法

2. **模型不可用时禁止预测**
   - 如果模型未加载或调用失败，直接抛出错误
   - 不返回任何预测结果（包括错误状态的结果）
   - 避免误导交易系统做出错误决策

3. **错误处理**
   - 所有预测失败都抛出 `RuntimeError`
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

### 错误处理

```python
except Exception as e:
    # 不返回任何预测结果
    raise RuntimeError(f"预测失败: {e}，禁止返回任何预测结果")
```

## ⚠️ 重要说明

1. **当前状态**：模型加载功能需要根据etg_ai项目实际结构实现
2. **临时行为**：当前会抛出 `NotImplementedError`，禁止任何预测
3. **集成要求**：必须实现真实的模型调用才能启用预测功能

## 🔧 如何启用预测

1. **设置etg_ai项目**：
   ```bash
   bash scripts/setup_etg_ai.sh
   ```

2. **实现模型加载**：
   - 在 `_load_model()` 中实现真实的模型加载
   - 确保 `self.model` 不为 `None`

3. **实现模型调用**：
   - 在 `_call_real_krnos_model()` 中实现真实的模型预测调用
   - 返回真实的模型预测结果

4. **测试验证**：
   - 确保模型可以正常加载和调用
   - 验证预测结果的合理性

## ✅ 验证清单

- [ ] etg_ai项目已设置
- [ ] 模型可以正常加载（`self.model` 不为 `None`）
- [ ] 模型可以正常调用（`_call_real_krnos_model()` 实现）
- [ ] 预测结果格式正确
- [ ] 错误处理正确（模型不可用时抛出错误）

## 🎯 目标

确保交易系统只使用真实的、经过验证的模型预测结果，避免任何模拟或备用方案导致的错误决策。

