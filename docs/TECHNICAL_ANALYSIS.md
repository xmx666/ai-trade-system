# NOFX 核心技术分析报告

## 📋 概述

本报告详细分析 NOFX AI交易系统的核心技术实现，包括Prompt系统、决策引擎、交易流程、市场数据分析等关键组件。

---

## 1. Prompt 系统架构

### 1.1 Prompt 模板管理系统

**位置**: `decision/prompt_manager.go`

**核心功能**:
- 自动扫描 `prompts/` 目录下的所有 `.txt` 文件
- 在包初始化时（`init()`）自动加载所有模板到内存
- 使用线程安全的 `sync.RWMutex` 保护并发访问
- 支持模板热重载（`ReloadTemplates()`）

**关键数据结构**:
```go
type PromptTemplate struct {
    Name    string // 模板名称（文件名，不含扩展名）
    Content string // 模板内容
}

type PromptManager struct {
    templates map[string]*PromptTemplate
    mu        sync.RWMutex
}
```

**模板加载流程**:
```
1. 系统启动时调用 init()
   ↓
2. 创建全局 PromptManager 实例
   ↓
3. 扫描 prompts/ 目录下的 *.txt 文件
   ↓
4. 读取每个文件内容，文件名（不含扩展名）作为模板名称
   ↓
5. 存储到内存中的 templates map
   ↓
6. 日志输出已加载的模板列表
```

**当前可用模板**:
- `default.txt` - 默认策略（最大化夏普比率）
- `scalping.txt` - 高频做T策略
- `trend_following.txt` - 趋势跟踪策略
- `adaptive.txt` - 自适应策略
- `nof1.txt` - NOF1策略
- `taro_long_prompts.txt` - 塔罗牌多头策略

### 1.2 System Prompt 构建流程

**位置**: `decision/engine.go` - `buildSystemPrompt()`

**构建逻辑**:
```
基础模板（从文件加载）
    ↓
+ 硬约束（风险控制规则，动态生成）
    ↓
+ 输出格式说明（JSON格式要求，动态生成）
    ↓
= 完整的 System Prompt
```

**动态生成部分**:

1. **硬约束（风险控制）**:
```go
sb.WriteString("# 硬约束（风险控制）\n\n")
sb.WriteString("1. 风险回报比: 必须 ≥ 1:3（冒1%风险，赚3%+收益）\n")
sb.WriteString("2. 最多持仓: 3个币种（质量>数量）\n")
sb.WriteString(fmt.Sprintf("3. 单币仓位: 山寨%.0f-%.0f U(%dx杠杆) | BTC/ETH %.0f-%.0f U(%dx杠杆)\n",
    accountEquity*0.8, accountEquity*1.5, altcoinLeverage, 
    accountEquity*5, accountEquity*10, btcEthLeverage))
sb.WriteString("4. 保证金: 总使用率 ≤ 90%\n\n")
```

2. **输出格式说明**:
```go
sb.WriteString("#输出格式\n\n")
sb.WriteString("第一步: 思维链（纯文本）\n")
sb.WriteString("第二步: JSON决策数组\n\n")
sb.WriteString("```json\n[\n")
sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, ...},\n", ...))
sb.WriteString("]\n```\n\n")
```

### 1.3 自定义 Prompt 支持

**位置**: `decision/engine.go` - `buildSystemPromptWithCustom()`

**支持模式**:
1. **覆盖模式** (`overrideBase = true`):
   - 完全使用自定义 Prompt，忽略基础模板
   - 适用于完全自定义策略

2. **补充模式** (`overrideBase = false`):
   - 基础模板 + 自定义 Prompt 作为补充
   - 自定义内容添加到基础模板后面
   - 标记为"个性化交易策略"，不能违背基础风险控制原则

**代码实现**:
```go
func buildSystemPromptWithCustom(accountEquity float64, btcEthLeverage, altcoinLeverage int, 
    customPrompt string, overrideBase bool, templateName string) string {
    
    // 覆盖模式：只使用自定义Prompt
    if overrideBase && customPrompt != "" {
        return customPrompt
    }
    
    // 获取基础模板
    basePrompt := buildSystemPrompt(accountEquity, btcEthLeverage, altcoinLeverage, templateName)
    
    // 如果没有自定义Prompt，直接返回基础模板
    if customPrompt == "" {
        return basePrompt
    }
    
    // 补充模式：基础模板 + 自定义内容
    var sb strings.Builder
    sb.WriteString(basePrompt)
    sb.WriteString("\n\n")
    sb.WriteString("# 📌 个性化交易策略\n\n")
    sb.WriteString(customPrompt)
    sb.WriteString("\n\n")
    sb.WriteString("注意: 以上个性化策略是对基础规则的补充，不能违背基础风险控制原则。\n")
    
    return sb.String()
}
```

### 1.4 User Prompt 构建流程

**位置**: `decision/engine.go` - `buildUserPrompt()`

**包含内容**:
1. **系统状态**:
   - 当前时间、周期编号、运行时长

2. **BTC 市场概览**:
   - 当前价格、1小时/4小时价格变化
   - MACD、RSI指标

3. **账户状态**:
   - 净值、可用余额、盈亏百分比
   - 保证金使用率、持仓数量

4. **当前持仓（完整市场数据）**:
   - 每个持仓的详细信息（入场价、当前价、盈亏、杠杆、保证金、强平价）
   - 持仓时长（从首次出现时间计算）
   - 完整的技术指标序列（3分钟和4小时）

5. **候选币种（完整市场数据）**:
   - 最多分析前20个候选币种（来自AI500或默认币种）
   - 每个币种的完整技术指标序列

6. **历史表现反馈**（如果可用）:
   - 最近20个周期的交易表现
   - 胜率、盈亏比、夏普比率
   - 最佳/最差币种统计
   - 最近5笔交易详情

---

## 2. 决策引擎核心流程

### 2.1 决策获取流程

**位置**: `decision/engine.go` - `GetFullDecisionWithCustomPrompt()`

**完整流程**:
```
1. 获取市场数据
   ↓ fetchMarketDataForContext()
   - 并发获取所有币种的市场数据（3分钟K线 + 4小时K线）
   - 计算技术指标（EMA20/50, MACD, RSI7/14, ATR）
   - 获取OI数据和资金费率
   ↓
2. 构建 System Prompt
   ↓ buildSystemPromptWithCustom()
   - 加载基础模板（从 prompts/ 目录）
   - 添加硬约束（风险控制规则）
   - 添加输出格式说明
   - 添加自定义Prompt（如果提供）
   ↓
3. 构建 User Prompt
   ↓ buildUserPrompt()
   - 账户状态
   - 持仓信息（完整市场数据）
   - 候选币种（完整市场数据）
   - 历史表现反馈
   ↓
4. 调用 AI API
   ↓ mcpClient.CallWithMessages()
   - 使用 System Prompt + User Prompt
   - 超时时间：120秒
   ↓
5. 解析 AI 响应
   ↓ parseFullDecisionResponse()
   - 提取思维链（CoT trace）
   - 提取JSON决策数组
   - 验证决策格式
   ↓
6. 返回完整决策
   - System Prompt（用于日志）
   - User Prompt（用于日志）
   - CoT Trace（思维链）
   - Decisions（决策列表）
```

### 2.2 AI 响应解析机制

**位置**: `decision/engine.go` - `parseFullDecisionResponse()`

**解析流程**:
```
1. 提取思维链
   ↓ extractCoTTrace()
   - 查找第一个 `[` 字符
   - `[` 之前的所有内容都是思维链
   ↓
2. 提取JSON决策数组
   ↓ extractDecisions()
   - 查找第一个 `[` 字符
   - 使用 findMatchingBracket() 找到匹配的 `]`
   - 提取 `[` 和 `]` 之间的内容
   - 修复常见JSON格式错误（fixMissingQuotes）
   - JSON解析为 []Decision 数组
   ↓
3. 验证决策
   ↓ validateDecisions()
   - 验证 action 字段（必须是有效值）
   - 验证开仓必需字段（leverage, position_size_usd, stop_loss, take_profit等）
   - 验证杠杆倍数（不能超过配置上限）
   - 验证仓位大小（不能超过账户限制）
   - 验证止损止盈价格合理性
```

**关键函数**:
```go
// 查找匹配的右括号
func findMatchingBracket(s string, start int) int {
    depth := 0
    for i := start; i < len(s); i++ {
        switch s[i] {
        case '[':
            depth++
        case ']':
            depth--
            if depth == 0 {
                return i
            }
        }
    }
    return -1
}

// 修复中文引号等格式错误
func fixMissingQuotes(jsonStr string) string {
    jsonStr = strings.ReplaceAll(jsonStr, "\u201c", "\"") // "
    jsonStr = strings.ReplaceAll(jsonStr, "\u201d", "\"") // "
    return jsonStr
}
```

### 2.3 决策验证机制

**位置**: `decision/engine.go` - `validateDecision()`

**验证规则**:
1. **Action 验证**:
   - 必须是: `open_long`, `open_short`, `close_long`, `close_short`, `hold`, `wait`

2. **开仓必需字段**:
   - `leverage`: 杠杆倍数（1到配置上限）
   - `position_size_usd`: 仓位大小（>0）
   - `stop_loss`: 止损价格（>0）
   - `take_profit`: 止盈价格（>0）
   - `confidence`: 信心度（0-100）
   - `risk_usd`: 风险金额（>0）

3. **仓位限制验证**:
   - 山寨币：≤ 1.5倍账户净值
   - BTC/ETH：≤ 10倍账户净值

4. **杠杆限制验证**:
   - 根据币种类型使用对应的杠杆上限（BTCETHLeverage 或 AltcoinLeverage）

5. **止损止盈合理性**:
   - 做多：止损 < 止盈
   - 做空：止损 > 止盈

---

## 3. 交易执行流程

### 3.1 交易周期主循环

**位置**: `trader/auto_trader.go` - `Run()`

**执行流程**:
```
1. 初始化
   - 创建定时器（ScanInterval，默认3分钟）
   - 首次立即执行
   ↓
2. 定时循环
   for {
      等待定时器触发
      ↓
      执行 runCycle()
   }
```

### 3.2 单周期执行流程

**位置**: `trader/auto_trader.go` - `runCycle()`

**完整流程**:
```
1. 检查风险控制
   - 如果触发风控暂停，跳过本次周期
   ↓
2. 重置日盈亏（每天重置一次）
   ↓
3. 构建交易上下文
   ↓ buildTradingContext()
   - 获取账户状态
   - 获取持仓列表
   - 获取候选币种列表
   - 获取历史表现分析（最近20个周期）
   ↓
4. 调用AI获取决策
   ↓ GetFullDecisionWithCustomPrompt()
   - 使用指定的Prompt模板
   - 支持自定义Prompt
   ↓
5. 排序决策（先平仓后开仓）
   ↓ sortDecisionsByPriority()
   - 确保先执行平仓操作
   - 再执行开仓操作
   - 避免仓位叠加超限
   ↓
6. 执行决策
   ↓ executeDecision()
   - 根据action类型执行对应操作
   - 记录执行结果
   ↓
7. 记录决策日志
   ↓ decisionLogger.LogDecision()
   - 保存完整的决策上下文
   - 保存AI思维链
   - 保存执行结果
```

### 3.3 交易上下文构建

**位置**: `trader/auto_trader.go` - `buildTradingContext()`

**构建内容**:
```go
type Context struct {
    CurrentTime     string                  // 当前时间
    RuntimeMinutes  int                     // 运行时长（分钟）
    CallCount       int                     // AI调用次数
    Account         AccountInfo             // 账户信息
    Positions       []PositionInfo          // 持仓列表
    CandidateCoins  []CandidateCoin         // 候选币种列表
    MarketDataMap   map[string]*market.Data // 市场数据（3分钟+4小时）
    OITopDataMap    map[string]*OITopData  // OI Top数据
    Performance     interface{}             // 历史表现分析
    BTCETHLeverage  int                     // BTC/ETH杠杆配置
    AltcoinLeverage int                     // 山寨币杠杆配置
}
```

**关键步骤**:
1. **获取账户状态**:
   - 从交易所API获取实时账户信息
   - 计算总净值、可用余额、保证金使用率

2. **获取持仓列表**:
   - 从交易所API获取所有持仓
   - 计算未实现盈亏
   - 记录持仓首次出现时间（用于计算持仓时长）

3. **获取候选币种**:
   - 从币种池获取（AI500 + OI Top）
   - 或使用默认币种列表
   - 过滤低流动性币种（OI < 15M USD）

4. **获取历史表现**:
   - 从决策日志分析最近20个周期
   - 计算胜率、盈亏比、夏普比率
   - 识别最佳/最差币种

### 3.4 决策执行机制

**位置**: `trader/auto_trader.go` - `executeDecision()`

**执行优先级**:
1. **平仓优先**:
   - `close_long` / `close_short` 优先执行
   - 避免仓位叠加

2. **开仓次之**:
   - `open_long` / `open_short` 在平仓后执行

3. **等待/持有**:
   - `wait` / `hold` 不执行任何操作

**执行流程**:
```go
switch decision.Action {
case "open_long":
    // 1. 设置杠杆
    trader.SetLeverage(symbol, leverage)
    // 2. 设置仓位模式（全仓/逐仓）
    trader.SetMarginMode(symbol, isCrossMargin)
    // 3. 开多仓
    result := trader.OpenLong(symbol, quantity, leverage)
    // 4. 设置止损（如果提供）
    if stopLoss > 0 {
        trader.SetStopLoss(symbol, "LONG", quantity, stopLoss)
    }
    // 5. 设置止盈（如果提供）
    if takeProfit > 0 {
        trader.SetTakeProfit(symbol, "LONG", quantity, takeProfit)
    }
    
case "close_long":
    // 1. 平多仓（quantity=0表示全部平仓）
    result := trader.CloseLong(symbol, 0)
    // 2. 取消所有挂单
    trader.CancelAllOrders(symbol)
    
case "open_short":
    // 类似 open_long，但是开空仓
    
case "close_short":
    // 类似 close_long，但是平空仓
}
```

---

## 4. 市场数据分析系统

### 4.1 市场数据获取

**位置**: `market/data.go` - `Get()`

**数据源**:
- **WebSocket实时数据**: `market/monitor.go` - 实时K线数据缓存
- **Binance API**: 3分钟K线、4小时K线、OI数据、资金费率

**获取流程**:
```
1. 从WebSocket缓存获取3分钟K线（最近10根）
   ↓ WSMonitorCli.GetCurrentKlines(symbol, "3m")
   ↓
2. 从WebSocket缓存获取4小时K线（最近10根）
   ↓ WSMonitorCli.GetCurrentKlines(symbol, "4h")
   ↓
3. 计算技术指标
   - 3分钟K线：EMA20, MACD, RSI7, RSI14
   - 4小时K线：EMA20, EMA50, MACD, RSI14, ATR
   ↓
4. 获取OI数据（从Binance API）
   ↓ getOpenInterestData()
   ↓
5. 获取资金费率（从Binance API）
   ↓ getFundingRate()
   ↓
6. 计算价格变化百分比
   - 1小时变化：20个3分钟K线前的价格
   - 4小时变化：1个4小时K线前的价格
   ↓
7. 计算序列数据
   - 日内序列：3分钟价格序列、EMA20序列、MACD序列、RSI序列
   - 长期序列：4小时MACD序列、RSI序列
```

### 4.2 技术指标计算

**使用库**: TA-Lib (go-talib)

**计算的指标**:
1. **EMA (指数移动平均)**:
   - EMA20（3分钟K线）
   - EMA20/EMA50（4小时K线）

2. **MACD (指数平滑异同移动平均线)**:
   - 参数：12, 26, 9
   - 计算MACD线、信号线、柱状图

3. **RSI (相对强弱指标)**:
   - RSI7（3分钟K线，7周期）
   - RSI14（3分钟K线和4小时K线，14周期）

4. **ATR (平均真实波幅)**:
   - ATR3 / ATR14（4小时K线）

**序列数据生成**:
- 为每个时间点计算指标值，生成完整的序列
- AI可以分析序列的变化趋势，而不是只看当前值

### 4.3 WebSocket实时数据监控

**位置**: `market/monitor.go`

**核心功能**:
- 单连接多币种订阅（`combined_streams.go`）
- 自动重连机制
- 数据缓存和更新
- 实时K线数据维护

**数据缓存结构**:
```go
type WSMonitor struct {
    klines map[string]map[string][]Kline // symbol -> interval -> klines
    mu     sync.RWMutex
}
```

---

## 5. 性能分析系统

### 5.1 决策日志记录

**位置**: `logger/decision_logger.go` - `LogDecision()`

**记录内容**:
```go
type DecisionRecord struct {
    Timestamp      time.Time          // 决策时间
    CycleNumber    int                // 周期编号
    SystemPrompt   string             // 系统提示词
    InputPrompt    string             // 输入提示词
    CoTTrace       string             // AI思维链
    DecisionJSON   string             // 决策JSON
    AccountState   AccountSnapshot    // 账户状态快照
    Positions      []PositionSnapshot // 持仓快照
    CandidateCoins []string           // 候选币种列表
    Decisions      []DecisionAction   // 执行的决策
    ExecutionLog   []string           // 执行日志
    Success        bool               // 是否成功
    ErrorMessage   string             // 错误信息
}
```

**文件命名格式**:
```
decision_YYYYMMDD_HHMMSS_cycleN.json
例如: decision_20251104_100303_cycle2.json
```

### 5.2 性能分析算法

**位置**: `logger/decision_logger.go` - `AnalyzePerformance()`

**分析流程**:
```
1. 读取最近N个周期的决策记录
   ↓
2. 构建持仓状态追踪
   - 使用 symbol_side 作为key（如 "BTCUSDT_long"）
   - 追踪开仓时间、价格、数量、杠杆
   ↓
3. 匹配开仓和平仓记录
   - 遍历所有决策记录
   - 找到对应的开仓和平仓配对
   ↓
4. 计算盈亏
   - USDT盈亏：quantity × (closePrice - openPrice) × 方向
   - 百分比盈亏：(PnL / MarginUsed) × 100
   - 持仓时长：closeTime - openTime
   ↓
5. 统计指标
   - 总交易数、盈利交易数、亏损交易数
   - 胜率、平均盈利、平均亏损
   - 盈亏比（Profit Factor）
   - 夏普比率（Sharpe Ratio）
   ↓
6. 币种级别统计
   - 每个币种的交易次数、胜率、平均盈亏
   - 识别最佳/最差币种
```

**夏普比率计算**:
```go
func calculateSharpeRatio(records []*DecisionRecord) float64 {
    // 1. 提取每个周期的账户净值
    var equities []float64
    for _, record := range records {
        equities = append(equities, record.AccountState.TotalBalance)
    }
    
    // 2. 计算周期收益率
    var returns []float64
    for i := 1; i < len(equities); i++ {
        periodReturn := (equities[i] - equities[i-1]) / equities[i-1]
        returns = append(returns, periodReturn)
    }
    
    // 3. 计算平均收益率和标准差
    meanReturn := 平均值(returns)
    stdDev := 标准差(returns)
    
    // 4. 计算夏普比率
    sharpeRatio := meanReturn / stdDev
    
    return sharpeRatio
}
```

### 5.3 历史反馈注入

**位置**: `decision/engine.go` - `buildUserPrompt()`

**反馈内容**:
```
## 📊 Historical Performance Feedback

### Overall Performance
- Total Trades: 15 (Profit: 8 | Loss: 7)
- Win Rate: 53.3%
- Average Profit: +3.2% | Average Loss: -2.1%
- Profit/Loss Ratio: 1.52:1
- Sharpe Ratio: 0.35

### Recent Trades
1. BTCUSDT LONG: 95000.0000 → 97500.0000 = +2.63% ✓
2. ETHUSDT SHORT: 3500.0000 → 3450.0000 = +1.43% ✓
...

### Coin Performance
- Best: BTCUSDT (Win rate 75%, avg +2.5%)
- Worst: SOLUSDT (Win rate 25%, avg -1.8%)
```

---

## 6. 关键工具和组件

### 6.1 多交易所统一接口

**位置**: `trader/interface.go`

**设计模式**: 策略模式 + 接口抽象

**核心接口**:
```go
type Trader interface {
    GetBalance() (map[string]interface{}, error)
    GetPositions() ([]map[string]interface{}, error)
    OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error)
    OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error)
    CloseLong(symbol string, quantity float64) (map[string]interface{}, error)
    CloseShort(symbol string, quantity float64) (map[string]interface{}, error)
    SetLeverage(symbol string, leverage int) error
    SetMarginMode(symbol string, isCrossMargin bool) error
    SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error
    SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error
    CancelAllOrders(symbol string) error
    FormatQuantity(symbol string, quantity float64) (string, error)
}
```

**实现类**:
- `binance_futures.go` - Binance期货交易所
- `hyperliquid_trader.go` - Hyperliquid DEX
- `aster_trader.go` - Aster DEX

### 6.2 AI客户端（MCP）

**位置**: `mcp/client.go`

**支持的AI模型**:
1. **DeepSeek**:
   - 默认URL: `https://api.deepseek.com/v1`
   - 默认模型: `deepseek-chat`
   - 支持自定义URL和模型名

2. **Qwen (通义千问)**:
   - 默认URL: `https://dashscope.aliyuncs.com/compatible-mode/v1`
   - 默认模型: `qwen-plus`
   - 支持自定义URL和模型名

3. **自定义OpenAI兼容API**:
   - 支持任意OpenAI兼容的API
   - 自动检测URL格式（是否完整URL）
   - 支持GPT-4、Claude等

**API调用**:
```go
func (client *Client) CallWithMessages(systemPrompt, userPrompt string) (string, error) {
    // 构建请求
    request := ChatCompletionRequest{
        Model: client.Model,
        Messages: []ChatMessage{
            {Role: "system", Content: systemPrompt},
            {Role: "user", Content: userPrompt},
        },
        Temperature: 0.7,
    }
    
    // 发送HTTP请求
    // 解析响应
    // 返回AI回复内容
}
```

### 6.3 币种池管理

**位置**: `pool/coin_pool.go`

**币种来源**:
1. **默认币种列表**:
   - BTCUSDT, ETHUSDT, SOLUSDT, BNBUSDT, XRPUSDT, DOGEUSDT, ADAUSDT, HYPEUSDT

2. **AI500 API**:
   - 获取AI评分最高的20个币种

3. **OI Top API**:
   - 获取持仓量增长最快的20个币种

**合并逻辑**:
- 合并AI500和OI Top列表
- 去重（按symbol）
- 过滤低流动性币种（OI < 15M USD）

### 6.4 交易员管理器

**位置**: `manager/trader_manager.go`

**核心功能**:
- 管理多个交易员实例（map[string]*AutoTrader）
- 线程安全（使用 sync.RWMutex）
- 从数据库加载交易员配置
- 支持动态添加/删除交易员
- 提供竞赛数据（多AI对比）

---

## 7. 流程设计图

### 7.1 完整交易周期流程

```
┌─────────────────────────────────────────────────────────┐
│                   交易周期开始                            │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 1. 检查风险控制                                          │
│    - 是否在暂停期？                                      │
│    - 是否触发日亏损/回撤限制？                            │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 2. 构建交易上下文                                        │
│    - 获取账户状态（交易所API）                            │
│    - 获取持仓列表（交易所API）                            │
│    - 获取候选币种（币种池）                               │
│    - 获取历史表现（决策日志分析）                         │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 3. 获取市场数据（并发）                                   │
│    - 所有币种的3分钟K线（WebSocket缓存）                  │
│    - 所有币种的4小时K线（WebSocket缓存）                  │
│    - 计算技术指标（TA-Lib）                              │
│    - 获取OI数据和资金费率（API）                         │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 4. 构建Prompt                                           │
│    - System Prompt: 基础模板 + 硬约束 + 输出格式          │
│    - User Prompt: 账户状态 + 持仓 + 市场数据 + 历史反馈   │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 5. 调用AI API                                           │
│    - 使用System Prompt + User Prompt                     │
│    - 超时时间：120秒                                     │
│    - 返回：思维链 + JSON决策数组                          │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 6. 解析AI响应                                           │
│    - 提取思维链（CoT trace）                             │
│    - 提取JSON决策数组                                    │
│    - 验证决策格式和字段                                  │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 7. 排序决策（先平仓后开仓）                               │
│    - close_long, close_short 优先                        │
│    - open_long, open_short 次之                          │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 8. 执行决策（顺序执行）                                   │
│    - 平仓：CloseLong/CloseShort + CancelAllOrders        │
│    - 开仓：SetLeverage + SetMarginMode + OpenLong/Short  │
│    - 设置止损止盈：SetStopLoss + SetTakeProfit           │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 9. 记录决策日志                                         │
│    - 保存完整上下文（JSON文件）                           │
│    - 更新性能分析数据库                                   │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 10. 等待下一个周期                                       │
│     - 定时器触发（默认3分钟）                             │
└─────────────────────────────────────────────────────────┘
```

### 7.2 Prompt构建流程图

```
┌─────────────────────────────────────────────────────────┐
│ 开始构建System Prompt                                    │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 检查是否覆盖模式？                                       │
│ - 是：只使用自定义Prompt                                 │
│ - 否：继续                                              │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 加载基础模板                                             │
│ - 从 prompts/ 目录加载指定模板                           │
│ - 如果模板不存在，使用 default 模板                      │
│ - 如果都不存在，使用内置简化版本                         │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 添加硬约束（动态生成）                                    │
│ - 风险回报比要求                                         │
│ - 最多持仓数量                                           │
│ - 单币仓位限制（根据账户净值和杠杆计算）                  │
│ - 保证金使用率限制                                       │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 添加输出格式说明（动态生成）                               │
│ - 思维链格式要求                                         │
│ - JSON数组格式要求                                       │
│ - 字段说明和示例                                         │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 添加自定义Prompt（如果提供）                              │
│ - 补充模式：追加到基础Prompt后面                         │
│ - 标记为"个性化交易策略"                                  │
└─────────────────────────────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────┐
│ 返回完整的System Prompt                                  │
└─────────────────────────────────────────────────────────┘
```

---

## 8. 关键技术细节

### 8.1 并发安全设计

**使用场景**:
1. **PromptManager**: `sync.RWMutex` 保护模板读取
2. **TraderManager**: `sync.RWMutex` 保护交易员实例
3. **WSMonitor**: `sync.RWMutex` 保护K线数据缓存

**设计原则**:
- 读多写少场景使用 `RWMutex`
- 写操作使用 `Lock()`，读操作使用 `RLock()`
- 避免死锁（按固定顺序获取锁）

### 8.2 错误处理和降级

**错误处理策略**:
1. **市场数据获取失败**:
   - OI数据获取失败不影响整体（使用默认值）
   - 单个币种数据获取失败跳过该币种

2. **AI API调用失败**:
   - 记录完整错误信息
   - 保存已获取的思维链（如果有）
   - 跳过本次周期，等待下次

3. **决策解析失败**:
   - 记录失败原因和原始响应
   - 保存思维链用于调试
   - 跳过执行，等待下次

### 8.3 性能优化

**优化措施**:
1. **并发获取市场数据**:
   - 使用goroutine并发获取多个币种数据
   - 使用channel收集结果

2. **数据缓存**:
   - WebSocket实时数据缓存
   - 决策日志文件缓存（避免重复读取）

3. **批量处理**:
   - 批量获取市场数据
   - 批量计算技术指标

### 8.4 数据精度处理

**自动精度处理**:
- 从交易所API获取 `LOT_SIZE` 和 `PRICE_FILTER` 规则
- 自动格式化订单数量和价格
- 使用 `FormatQuantity()` 方法确保精度正确

**实现位置**: 各交易所实现类（`binance_futures.go`, `hyperliquid_trader.go`, `aster_trader.go`）

---

## 9. 总结

### 9.1 核心技术特点

1. **模块化设计**:
   - 各模块职责清晰，高内聚低耦合
   - 使用接口抽象实现多态

2. **灵活配置**:
   - Prompt模板系统支持多种策略
   - 支持自定义Prompt和模板选择
   - 数据库驱动的配置系统

3. **实时数据分析**:
   - WebSocket实时数据缓存
   - 多时间周期技术指标计算
   - 完整的历史表现反馈

4. **智能决策**:
   - AI思维链推理
   - 多维度市场分析
   - 自动风险控制

5. **完整日志**:
   - 决策上下文完整记录
   - 性能分析自动计算
   - 支持策略优化和调试

### 9.2 技术优势

1. **可扩展性**:
   - 易于添加新的交易所实现
   - 易于添加新的AI模型
   - 易于添加新的Prompt模板

2. **可维护性**:
   - 代码结构清晰
   - 完整的日志记录
   - 错误处理完善

3. **可测试性**:
   - 接口抽象便于Mock
   - 模块独立便于单元测试

4. **可观测性**:
   - 完整的决策日志
   - 实时性能分析
   - 思维链可追溯

---

## 10. 参考资料

- **Prompt模板位置**: `prompts/` 目录
- **决策引擎**: `decision/engine.go`
- **交易执行**: `trader/auto_trader.go`
- **市场数据**: `market/data.go`
- **性能分析**: `logger/decision_logger.go`
- **API文档**: `docs/architecture/README.zh-CN.md`

---

**报告生成时间**: 2025-11-04
**代码版本**: v3.0.0+

