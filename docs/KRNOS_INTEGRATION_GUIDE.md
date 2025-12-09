# krnos模型预测集成指南

## 📋 项目整体运行流程

### 1. 系统启动流程 (`main.go`)

```
启动流程：
1. 初始化配置数据库 (config.db)
2. 同步 config.json 到数据库
3. 加载交易员配置
4. 启动 API 服务器 (Web管理界面)
5. 启动 WebSocket 行情监控
6. 等待退出信号
```

### 2. 交易循环流程 (`trader/auto_trader.go`)

```
主循环 (Run):
  ↓
每3分钟触发一次 runCycle()
  ↓
runCycle() 执行流程:
  1. 检查风险控制（是否暂停交易）
  2. 重置日盈亏（每天重置）
  3. 检测持仓变化
  4. 检查并调整移动止损
  5. 构建交易上下文 (buildTradingContext)
     - 获取账户状态
     - 获取持仓列表
     - 获取候选币种列表
     - 获取历史表现分析
  6. 调用AI获取决策 (GetFullDecision)
     - fetchMarketDataForContext: 获取市场数据
     - buildSystemPrompt: 构建系统提示词
     - buildUserPrompt: 构建用户提示词（包含市场数据）
     - 调用AI API
     - 解析AI响应
  7. 排序决策（先平仓后开仓）
  8. 执行决策
  9. 记录决策日志
```

### 3. 决策引擎流程 (`decision/engine.go`)

```
GetFullDecision():
  1. fetchMarketDataForContext()
     - 获取持仓币种的市场数据
     - 获取候选币种的市场数据
     - 获取OI Top数据
     - ⚠️ 需要添加：获取krnos预测数据
  2. buildSystemPrompt()
     - 加载策略模板 (trend_following.txt)
     - 添加硬约束和输出格式
  3. buildUserPrompt()
     - 格式化账户信息
     - 格式化持仓信息
     - 格式化候选币种信息
     - 格式化历史表现
     - ⚠️ 需要添加：格式化预测数据
  4. 调用AI API
  5. 解析AI响应
```

## 🔧 krnos预测服务集成方案

### 集成点1: 在 `fetchMarketDataForContext()` 中获取预测数据

**位置**: `decision/engine.go` - `fetchMarketDataForContext()`

**修改内容**:
```go
// 在获取市场数据后，为每个币种获取预测数据
for symbol, data := range ctx.MarketDataMap {
    // 获取预测数据
    prediction, err := getPredictionForSymbol(symbol, data)
    if err == nil {
        ctx.PredictionMap[symbol] = prediction
    }
}
```

### 集成点2: 在 `buildUserPrompt()` 中显示预测数据

**位置**: `decision/engine.go` - `buildUserPrompt()`

**修改内容**:
```go
// 在显示市场数据时，同时显示预测数据
if prediction, ok := ctx.PredictionMap[symbol]; ok {
    sb.WriteString(formatPredictionData(prediction))
}
```

### 集成点3: 创建预测服务管理器

**新建文件**: `predictor/prediction_manager.go`

**功能**:
- 管理预测服务的生命周期
- 缓存预测结果
- 控制预测更新频率（30分钟一次）
- 处理预测失败的情况

## 📝 具体实现步骤

### 步骤1: 创建预测服务管理器

创建 `predictor/prediction_manager.go`:

```go
package predictor

import (
    "sync"
    "time"
)

var (
    globalPredictor *KrnosPredictorService
    predictorOnce   sync.Once
)

// GetGlobalPredictor 获取全局预测服务实例（单例模式）
func GetGlobalPredictor() *KrnosPredictorService {
    predictorOnce.Do(func() {
        globalPredictor = NewKrnosPredictorService()
    })
    return globalPredictor
}

// GetPredictionForSymbol 为指定币种获取预测数据
func GetPredictionForSymbol(symbol string, currentPrice float64, currentTrend string) (*PredictionData, error) {
    predictor := GetGlobalPredictor()
    
    // 检查是否需要重新预测
    if !predictor.ShouldRepredict(currentPrice, currentTrend) {
        // 使用缓存的预测结果
        latest, err := predictor.GetLatestPrediction()
        if err == nil {
            return convertToPredictionData(latest), nil
        }
    }
    
    // 需要重新预测（这里需要获取历史数据）
    // 实际实现需要从market包获取历史价格和成交量数据
    // 暂时返回nil，等待实际集成
    return nil, nil
}
```

### 步骤2: 修改 `decision/engine.go`

在 `fetchMarketDataForContext()` 中添加预测数据获取:

```go
// 获取krnos模型预测数据（为每个有市场数据的币种）
for symbol, data := range ctx.MarketDataMap {
    // 判断当前趋势（简化版，实际应该从技术指标判断）
    currentTrend := "sideways"
    if data.PriceChange1h > 0.5 {
        currentTrend = "up"
    } else if data.PriceChange1h < -0.5 {
        currentTrend = "down"
    }
    
    // 获取预测数据
    prediction, err := predictor.GetPredictionForSymbol(
        symbol,
        data.CurrentPrice,
        currentTrend,
    )
    if err == nil && prediction != nil {
        ctx.PredictionMap[symbol] = prediction
    }
}
```

### 步骤3: 修改 `buildUserPrompt()` 显示预测数据

在显示市场数据时添加预测信息:

```go
// 在 FormatMarketData 之后添加预测数据
if prediction, ok := ctx.PredictionMap[symbol]; ok {
    sb.WriteString("\n**krnos模型预测**:\n")
    sb.WriteString(fmt.Sprintf("  趋势: %s | 强度: %.2f | 价格范围: [%.2f, %.2f]\n",
        prediction.Trend, prediction.TrendStrength,
        prediction.PriceRange[0], prediction.PriceRange[1]))
}
```

### 步骤4: 在 `main.go` 中初始化预测服务（可选）

如果需要提前初始化:

```go
// 在启动API服务器之前
log.Println("🔮 初始化krnos预测服务...")
predictor.GetGlobalPredictor() // 触发单例初始化
log.Println("✓ krnos预测服务已就绪")
```

## 🚀 运行流程（集成后）

```
1. 系统启动
   ↓
2. 初始化预测服务（单例模式）
   ↓
3. 启动交易循环
   ↓
4. 每3分钟执行一次决策周期
   ↓
5. 构建交易上下文
   - 获取市场数据
   - 获取预测数据（每30分钟更新一次，或当预测与市场差异大时）
   ↓
6. 构建用户提示词
   - 包含市场数据
   - 包含预测数据
   ↓
7. AI决策（考虑预测数据）
   ↓
8. 执行交易
```

## ⚙️ 配置说明

### 预测更新策略

- **定时更新**: 每30分钟自动更新一次
- **差异触发**: 当以下情况发生时，自动重新预测：
  - 当前价格超出预测价格范围的5%
  - 预测趋势与当前市场趋势相反
  - 预测结果超过1小时未更新

### 性能优化

- **蒙特卡罗模拟次数**: 限制在10次以内
- **预测缓存**: 使用最新预测结果，避免频繁调用
- **异步预测**: 预测在后台进行，不阻塞主交易循环

## 📊 预测数据格式

预测数据将包含在AI的决策上下文中：

```
**krnos模型预测**:
  趋势: up/down/sideways
  趋势强度: 0.75 (0-1之间)
  价格范围: [90000, 92000]
  置信区间: [89500, 92500]
  预测时间: 2024-01-01 12:00:00
```

AI将根据这些预测数据，结合技术指标，做出更准确的交易决策。

## 🔍 调试和监控

### 查看预测日志

预测服务的日志会输出到标准输出，包括：
- 预测更新时机
- 预测结果
- 错误信息

### 检查预测缓存

预测结果保存在 `./predictor_cache/latest_prediction.json`

### 验证预测集成

在AI的思维链中，应该能看到对预测数据的引用和分析。

## ⚠️ 注意事项

1. **模型依赖**: 需要先设置etg_ai项目并下载krnos模型
2. **性能影响**: 预测服务在后台运行，不会阻塞主交易循环
3. **错误处理**: 如果预测服务失败，系统会继续运行，只是没有预测数据
4. **数据质量**: 预测结果依赖于历史数据的质量

## 🎯 下一步

1. 根据etg_ai项目的实际结构，实现 `GetPredictionForSymbol()` 函数
2. 从market包获取历史价格和成交量数据
3. 测试预测服务的集成
4. 监控预测对交易决策的影响

