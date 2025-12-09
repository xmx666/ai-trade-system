# krnos模型预测集成指南

## 概述

本文档说明如何将etg_ai项目的krnos模型预测集成到trend_following策略中，作为重要的趋势判断参考指标。

## 功能特点

1. **智能预测更新**：
   - 每30分钟自动更新一次预测
   - 当预测结果与当前市场有较大差异时，自动重新生成预测
   - 使用蒙特卡罗方法进行范围预测（不超过10次模拟以降低GPU压力和预测时间），提供置信区间

2. **预测内容**：
   - 预测趋势：'up'（上涨）、'down'（下跌）、'sideways'（横盘）
   - 预测曲线：未来24个周期（72分钟）的平均价格预测
   - 置信区间：95%置信区间的上下限
   - 价格范围：预测的价格范围 [最低价, 最高价]
   - 趋势强度：0-1之间的数值，表示预测趋势的强度

3. **策略应用**：
   - 作为重要的参考指标，权重20%
   - 与技术指标结合使用，提高判断准确性
   - 如果预测趋势与技术指标一致，提高信心度
   - 如果预测趋势与技术指标相反，需要更多确认

## 安装步骤

### 1. 设置etg_ai项目

```bash
# 运行设置脚本
./scripts/setup_etg_ai.sh
```

或者手动克隆项目：

```bash
cd ..
git clone https://github.com/your-username/etg_ai.git
cd etg_ai
pip install -r requirements.txt
```

### 2. 下载krnos模型

根据etg_ai项目的README，下载krnos模型到 `./models/krnos/` 目录。

### 3. 安装Python依赖

```bash
pip install numpy
# 根据etg_ai项目的实际需求安装其他依赖
```

## 使用方法

### Python脚本方式

```python
from predictor.krnos_predictor import KrnosPredictor

# 初始化预测器
predictor = KrnosPredictor()

# 准备历史数据
price_history = [...]  # 价格序列
volume_history = [...]  # 成交量序列

# 进行预测
result = predictor.predict_with_monte_carlo(
    price_history=price_history,
    volume_history=volume_history,
    n_simulations=10,  # 蒙特卡罗模拟次数（不超过10次以降低GPU压力和预测时间）
    prediction_horizon=24  # 预测未来24个周期
)

# 获取预测结果
print(f"预测趋势: {result['trend']}")
print(f"趋势强度: {result['trend_strength']}")
print(f"价格范围: {result['price_range']}")
```

### Go服务方式

```go
import "nofx/predictor"

// 创建预测服务
service := predictor.NewKrnosPredictorService()

// 判断是否需要重新预测
shouldRepredict := service.ShouldRepredict(currentPrice, currentTrend)

if shouldRepredict {
    // 进行预测
    result, err := service.Predict(
        priceHistory,
        volumeHistory,
        1000,  // 蒙特卡罗模拟次数
        24,    // 预测未来24个周期
    )
    if err != nil {
        log.Printf("预测失败: %v", err)
    } else {
        log.Printf("预测趋势: %s, 趋势强度: %.2f", result.Trend, result.TrendStrength)
    }
}

// 获取最新预测
latest, err := service.GetLatestPrediction()
if err == nil {
    // 使用预测结果进行判断
    trend, strength, _ := service.GetPredictionTrend()
}
```

## 策略中的应用

### 1. 趋势判断

在策略的"市场环境判断"部分，首先检查krnos模型预测：

- 如果预测趋势为'up'且趋势强度 > 0.7 → 支持做多
- 如果预测趋势为'down'且趋势强度 > 0.7 → 支持做空
- 如果预测趋势为'sideways'或趋势强度 < 0.3 → 市场方向不明确，谨慎交易

### 2. 综合评分

在综合评分系统中，krnos模型预测占20%权重：

- 预测趋势与技术指标一致：+10分（趋势强度 > 0.7时）
- 预测趋势与技术指标一致：+5分（趋势强度 0.3-0.7时）
- 预测趋势与技术指标相反：-10分（需要更多确认）

### 3. 价格范围判断

- 如果当前价格超出预测价格范围 → 可能需要重新预测，谨慎交易
- 如果当前价格接近范围下限 → 可能反弹（做多机会）
- 如果当前价格接近范围上限 → 可能回调（做空机会）

## 预测更新策略

1. **定时更新**：每30分钟自动更新一次预测
2. **差异触发**：当以下情况发生时，自动重新预测：
   - 当前价格超出预测价格范围的5%
   - 预测趋势与当前市场趋势相反
   - 预测结果超过1小时未更新

## 注意事项

1. **模型依赖**：需要先设置etg_ai项目并下载krnos模型
2. **数据质量**：预测结果依赖于历史数据的质量，确保输入数据准确
3. **计算资源**：蒙特卡罗模拟限制在10次以内以降低GPU压力和预测时间，建议在后台运行
4. **预测可靠性**：预测结果仅供参考，需要与技术指标结合使用
5. **不要过度依赖**：预测模型可能出错，不要完全依赖预测结果

## 故障排除

### 问题1：无法导入etg_ai项目

**解决方案**：
1. 确保etg_ai项目已克隆到 `../etg_ai` 目录
2. 检查Python路径是否正确
3. 根据etg_ai项目的实际结构调整导入语句

### 问题2：模型文件未找到

**解决方案**：
1. 检查模型文件是否在 `./models/krnos/` 目录
2. 根据etg_ai项目的README下载模型
3. 更新 `krnos_predictor.py` 中的模型加载路径

### 问题3：预测结果不准确

**解决方案**：
1. 检查输入数据的质量和长度
2. 蒙特卡罗模拟次数已限制在10次以内，如需调整请修改代码中的限制
3. 检查模型是否需要重新训练或更新

## 未来改进

1. **实时预测**：集成到交易系统中，实时更新预测
2. **多币种支持**：支持多个币种的预测
3. **预测历史**：保存预测历史，用于回测和分析
4. **性能优化**：优化预测速度，减少计算时间
5. **可视化**：添加预测曲线的可视化功能

