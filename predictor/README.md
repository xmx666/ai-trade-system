# krnos模型预测集成

## 快速开始

### 1. 设置etg_ai项目

```bash
# 运行设置脚本
./scripts/setup_etg_ai.sh
```

或者手动操作：

```bash
# 克隆etg_ai项目（请替换为实际的GitHub URL）
cd ..
git clone https://github.com/your-username/etg_ai.git
cd etg_ai
pip install -r requirements.txt

# 下载krnos模型到 nofx/models/krnos/ 目录
# 根据etg_ai项目的README操作
```

### 2. 安装Python依赖

```bash
pip install numpy
# 根据etg_ai项目的实际需求安装其他依赖
```

### 3. 测试预测功能

```bash
# 运行Python测试
python3 predictor/krnos_predictor.py
```

## 功能说明

### 预测更新策略

1. **定时更新**：每30分钟自动更新一次预测
2. **差异触发**：当以下情况发生时，自动重新预测：
   - 当前价格超出预测价格范围的5%
   - 预测趋势与当前市场趋势相反
   - 预测结果超过1小时未更新

### 预测内容

- **预测趋势**：'up'（上涨）、'down'（下跌）、'sideways'（横盘）
- **预测曲线**：未来120步的平均价格预测（标准预测方式）
- **置信区间**：95%置信区间的上下限（使用蒙特卡罗方法生成）
- **价格范围**：预测的价格范围 [最低价, 最高价]
- **趋势强度**：0-1之间的数值，表示预测趋势的强度

### 在策略中的应用

krnos模型预测已集成到 `prompts/trend_following.txt` 策略文件中，作为重要的参考指标（权重20%）。

**使用原则**：
- 如果预测趋势与技术指标一致 → 提高信心度（+10分）
- 如果预测趋势与技术指标相反 → 降低信心度（-10分），需要更多确认
- 如果预测趋势强度 > 0.7 → 预测可靠性高，权重提高
- 如果预测趋势强度 < 0.3 → 预测可靠性低，权重降低

## 代码结构

```
predictor/
├── krnos_predictor.py      # Python预测服务（主要实现）
├── prediction_service.go   # Go预测服务（接口）
├── integration_example.go  # 集成示例
└── README.md              # 本文档
```

## 集成到交易系统

### Python方式

```python
from predictor.krnos_predictor import KrnosPredictor

predictor = KrnosPredictor()
result = predictor.predict_with_monte_carlo(
    price_history=[...],
    volume_history=[...],
    n_simulations=10,  # 不超过10次以降低GPU压力和预测时间
    prediction_horizon=120  # 标准预测120步
)
```

### Go方式

```go
import "nofx/predictor"

service := predictor.NewKrnosPredictorService()
result, err := service.Predict(priceHistory, volumeHistory, 10, 120) // 标准预测120步
```

## 注意事项

1. **模型依赖**：需要先设置etg_ai项目并下载krnos模型
2. **数据质量**：预测结果依赖于历史数据的质量
3. **计算资源**：蒙特卡罗模拟需要一定的计算资源
4. **预测可靠性**：预测结果仅供参考，需要与技术指标结合使用

## 故障排除

### 无法导入etg_ai项目

1. 确保etg_ai项目已克隆到 `../etg_ai` 目录
2. 检查Python路径是否正确
3. 根据etg_ai项目的实际结构调整 `krnos_predictor.py` 中的导入语句

### 模型文件未找到

1. 检查模型文件是否在 `./models/krnos/` 目录
2. 根据etg_ai项目的README下载模型
3. 更新 `krnos_predictor.py` 中的模型加载路径

## 下一步

1. 根据etg_ai项目的实际结构，修改 `krnos_predictor.py` 中的模型加载和预测逻辑
2. 在交易系统中集成预测服务，实时更新预测
3. 根据实际使用情况调整预测更新策略和参数

