# NOFX项目运行流程和krnos集成说明

## 📋 项目整体架构

```
nofx项目
├── main.go                    # 主程序入口
├── trader/                    # 交易模块
│   └── auto_trader.go         # 自动交易主循环
├── decision/                  # 决策模块
│   ├── engine.go              # 决策引擎（AI调用）
│   └── prompt_manager.go      # 提示词管理
├── market/                    # 市场数据模块
│   └── monitor.go             # WebSocket行情监控
├── predictor/                 # krnos预测模块（新增）
│   ├── krnos_predictor.py     # Python预测服务
│   ├── prediction_service.go  # Go预测服务接口
│   └── prediction_manager.go  # 预测服务管理器
├── prompts/                   # 策略提示词
│   └── trend_following.txt    # 趋势跟踪策略（已集成预测说明）
└── api/                       # Web管理界面
    └── server.go              # API服务器
```

## 🚀 系统启动流程

### 1. 主程序启动 (`main.go`)

```go
main() {
    1. 初始化配置数据库 (config.db)
    2. 同步 config.json 到数据库
    3. 加载交易员配置
    4. 启动 API 服务器 (Web管理界面，端口8080)
    5. 启动 WebSocket 行情监控
    6. 等待退出信号 (Ctrl+C)
}
```

### 2. 交易循环启动

交易员通过Web界面启动后，会执行：

```go
AutoTrader.Run() {
    创建定时器（每3分钟触发一次）
    ↓
    首次立即执行 runCycle()
    ↓
    循环执行 runCycle()
}
```

## 🔄 单周期执行流程 (`runCycle()`)

```
每3分钟执行一次：

1. 检查风险控制
   └─ 如果触发风控暂停，跳过本次周期

2. 重置日盈亏（每天重置一次）

3. 检测持仓变化
   └─ 自动记录止损/止盈触发的平仓

4. 检查并调整移动止损
   └─ 根据盈利情况动态调整止损价格

5. 构建交易上下文 (buildTradingContext)
   ├─ 获取账户状态
   ├─ 获取持仓列表
   ├─ 获取候选币种列表
   └─ 获取历史表现分析

6. 调用AI获取决策 (GetFullDecision)
   ├─ fetchMarketDataForContext()
   │   ├─ 获取市场数据（价格、技术指标等）
   │   ├─ 获取OI Top数据
   │   └─ ⚠️ 获取krnos预测数据（新增）
   ├─ buildSystemPrompt()
   │   └─ 加载策略模板 (trend_following.txt)
   ├─ buildUserPrompt()
   │   ├─ 格式化账户信息
   │   ├─ 格式化持仓信息
   │   ├─ 格式化候选币种信息
   │   ├─ 格式化历史表现
   │   └─ ⚠️ 格式化预测数据（新增）
   ├─ 调用AI API
   └─ 解析AI响应

7. 排序决策（先平仓后开仓）

8. 执行决策
   └─ 根据action类型执行对应操作

9. 记录决策日志
   └─ 保存完整的决策上下文和执行结果
```

## 🔮 krnos预测服务集成

### 集成点1: 预测服务初始化

**位置**: `predictor/prediction_manager.go`

**实现**: 单例模式，首次调用时自动初始化

```go
GetGlobalPredictor() {
    单例模式，确保整个系统只有一个预测服务实例
    自动初始化，无需手动调用
}
```

### 集成点2: 获取预测数据

**位置**: `decision/engine.go` - `fetchMarketDataForContext()`

**当前状态**: 已添加占位代码，等待实际实现

**需要实现**:
```go
// 在获取市场数据后，为每个币种获取预测数据
for symbol, data := range ctx.MarketDataMap {
    // 1. 从market包获取历史价格和成交量数据
    priceHistory := getPriceHistory(symbol, 100)  // 最近100个数据点
    volumeHistory := getVolumeHistory(symbol, 100)
    
    // 2. 判断当前趋势
    currentTrend := determineTrend(data)
    
    // 3. 获取预测数据
    prediction, err := predictor.GetPredictionForSymbol(
        symbol,
        data.CurrentPrice,
        currentTrend,
        priceHistory,
        volumeHistory,
    )
    
    // 4. 存储预测结果
    if err == nil && prediction != nil {
        ctx.PredictionMap[symbol] = prediction
    }
}
```

### 集成点3: 显示预测数据

**位置**: `decision/engine.go` - `buildUserPrompt()`

**已实现**: 在持仓和候选币种的市场数据后显示预测信息

**显示格式**:
```
**krnos模型预测**: 趋势=up | 强度=0.75 | 价格范围=[90000, 92000]
```

## 📊 预测更新策略

### 更新时机

1. **定时更新**: 每30分钟自动更新一次
2. **差异触发**: 当以下情况发生时，自动重新预测：
   - 当前价格超出预测价格范围的5%
   - 预测趋势与当前市场趋势相反
   - 预测结果超过1小时未更新

### 性能优化

- **蒙特卡罗模拟次数**: 限制在10次以内（降低GPU压力）
- **预测缓存**: 使用最新预测结果，避免频繁调用
- **异步预测**: 预测在后台进行，不阻塞主交易循环

## 🎯 实际使用步骤

### 步骤1: 设置etg_ai项目

```bash
# 运行设置脚本
./scripts/setup_etg_ai.sh

# 或手动克隆
cd ..
git clone https://github.com/your-username/etg_ai.git
cd etg_ai
pip install -r requirements.txt
```

### 步骤2: 下载krnos模型

根据etg_ai项目的README，下载模型到 `./models/krnos/` 目录

### 步骤3: 实现预测数据获取

在 `decision/engine.go` 的 `fetchMarketDataForContext()` 中：

1. 从market包获取历史数据
2. 调用 `predictor.GetPredictionForSymbol()` 获取预测
3. 将预测结果存储到 `ctx.PredictionMap`

### 步骤4: 测试集成

1. 启动系统
2. 查看日志，确认预测服务初始化成功
3. 查看AI决策日志，确认预测数据已包含在提示词中
4. 验证AI是否使用了预测数据进行决策

## 📝 代码修改清单

### 已完成的修改

✅ `decision/engine.go`:
- 添加 `PredictionData` 结构体
- 在 `Context` 中添加 `PredictionMap` 字段
- 在 `fetchMarketDataForContext()` 中添加预测数据获取占位代码
- 在 `buildUserPrompt()` 中添加预测数据显示

✅ `predictor/prediction_manager.go`:
- 创建预测服务管理器（单例模式）
- 实现 `GetPredictionForSymbol()` 接口

✅ `prompts/trend_following.txt`:
- 添加krnos预测的使用说明
- 添加预测数据的权重和评分规则

### 待完成的修改

⚠️ `decision/engine.go`:
- 实现从market包获取历史数据的函数
- 实现 `fetchMarketDataForContext()` 中的预测数据获取逻辑

⚠️ `predictor/krnos_predictor.py`:
- 根据etg_ai项目实际结构实现模型加载
- 实现实际的预测函数调用

## 🔍 调试和监控

### 查看预测日志

预测服务的日志会输出到标准输出：
```
🔮 krnos预测服务已初始化（单例模式）
⚠️  BTCUSDT 历史数据不足，跳过预测
✓ BTCUSDT 预测完成: 趋势=up, 强度=0.75
```

### 检查预测缓存

预测结果保存在：
- `./predictor_cache/latest_prediction.json` - 最新预测
- `./predictor_cache/prediction_YYYYMMDD_HHMMSS.json` - 历史预测

### 验证预测集成

在AI的思维链中，应该能看到：
```
**krnos模型预测**: 趋势=up | 强度=0.75 | 价格范围=[90000, 92000]
预测趋势与技术指标一致，提高信心度+10分
```

## ⚠️ 注意事项

1. **模型依赖**: 需要先设置etg_ai项目并下载krnos模型
2. **性能影响**: 预测服务在后台运行，不会阻塞主交易循环
3. **错误处理**: 如果预测服务失败，系统会继续运行，只是没有预测数据
4. **数据质量**: 预测结果依赖于历史数据的质量
5. **GPU资源**: 蒙特卡罗模拟限制在10次以内，但仍需注意GPU使用情况

## 🎯 下一步

1. 根据etg_ai项目的实际结构，实现模型加载和预测函数
2. 从market包获取历史价格和成交量数据
3. 完成 `fetchMarketDataForContext()` 中的预测数据获取逻辑
4. 测试预测服务的集成
5. 监控预测对交易决策的影响

