package decision

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"nofx/market"
	"nofx/mcp"
	"nofx/pool"
	"nofx/predictor"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PositionInfo 持仓信息
type PositionInfo struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"` // "long" or "short"
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	LiquidationPrice float64 `json:"liquidation_price"`
	MarginUsed       float64 `json:"margin_used"`
	UpdateTime       int64   `json:"update_time"` // 持仓更新时间戳（毫秒）
}

// AccountInfo 账户信息
type AccountInfo struct {
	TotalEquity      float64 `json:"total_equity"`      // 账户净值
	AvailableBalance float64 `json:"available_balance"` // 可用余额
	TotalPnL         float64 `json:"total_pnl"`         // 总盈亏
	TotalPnLPct      float64 `json:"total_pnl_pct"`     // 总盈亏百分比
	MarginUsed       float64 `json:"margin_used"`       // 已用保证金
	MarginUsedPct    float64 `json:"margin_used_pct"`   // 保证金使用率
	PositionCount    int     `json:"position_count"`    // 持仓数量
}

// CandidateCoin 候选币种（来自币种池）
type CandidateCoin struct {
	Symbol  string   `json:"symbol"`
	Sources []string `json:"sources"` // 来源: "ai500" 和/或 "oi_top"
}

// OITopData 持仓量增长Top数据（用于AI决策参考）
type OITopData struct {
	Rank              int     // OI Top排名
	OIDeltaPercent    float64 // 持仓量变化百分比（1小时）
	OIDeltaValue      float64 // 持仓量变化价值
	PriceDeltaPercent float64 // 价格变化百分比
	NetLong           float64 // 净多仓
	NetShort          float64 // 净空仓
}

// PredictionData krnos模型预测数据
// 注意：使用predictor包中的PredictionData类型，这里只是类型别名
type PredictionData = predictor.PredictionData

// AutoClosedPosition 自动平仓信息（止损/止盈触发）
type AutoClosedPosition struct {
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"` // "long" or "short"
	EntryPrice float64 `json:"entry_price"`
	ClosePrice float64 `json:"close_price"`
	Quantity  float64 `json:"quantity"`
	Leverage  int     `json:"leverage"`
	Reason    string  `json:"reason"` // "止损触发" or "止盈触发" or "未知"
}

// Context 交易上下文（传递给AI的完整信息）
type Context struct {
	CurrentTime     string                  `json:"current_time"`
	RuntimeMinutes  int                     `json:"runtime_minutes"`
	CallCount       int                     `json:"call_count"`
	TodayTradeCount int                     `json:"today_trade_count"` // 今天的交易次数（已废弃，使用HourlyTradeCount）
	HourlyTradeCount int                    `json:"hourly_trade_count"` // 最近1小时的交易次数
	HoursSinceLastTrade float64             `json:"hours_since_last_trade"` // 距离最近一次开单的小时数（如果没有开单记录则为-1）
	Account         AccountInfo             `json:"account"`
	Positions       []PositionInfo          `json:"positions"`
	CandidateCoins  []CandidateCoin         `json:"candidate_coins"`
	MarketDataMap   map[string]*market.Data `json:"-"` // 不序列化，但内部使用
	OITopDataMap    map[string]*OITopData   `json:"-"` // OI Top数据映射
	PredictionMap   map[string]*PredictionData `json:"-"` // krnos模型预测数据映射
	Performance     interface{}             `json:"-"` // 历史表现分析（logger.PerformanceAnalysis）
	BTCETHLeverage  int                     `json:"-"` // BTC/ETH杠杆倍数（从配置读取）
	AltcoinLeverage int                     `json:"-"` // 山寨币杠杆倍数（从配置读取）
	AutoClosedPositions []AutoClosedPosition `json:"-"` // 自动检测到的平仓信息（止损/止盈触发）
}

// Decision AI的交易决策
type Decision struct {
	Symbol          string  `json:"symbol"`
	Action          string  `json:"action"` // "open_long", "open_short", "close_long", "close_short", "hold", "wait"
	Leverage        int     `json:"leverage,omitempty"`
	PositionSizeUSD float64 `json:"position_size_usd,omitempty"`
	StopLoss        float64 `json:"stop_loss,omitempty"`
	TakeProfit      float64 `json:"take_profit,omitempty"`
	Confidence      int     `json:"confidence,omitempty"` // 信心度 (0-100)
	RiskUSD         float64 `json:"risk_usd,omitempty"`   // 最大美元风险
	Reasoning       string  `json:"reasoning"`
}

// FullDecision AI的完整决策（包含思维链）
type FullDecision struct {
	SystemPrompt string     `json:"system_prompt"`   // 系统提示词（发送给AI的系统prompt）
	UserPrompt   string     `json:"user_prompt"`     // 发送给AI的输入prompt
	CoTTrace     string     `json:"cot_trace"`       // 思维链分析（AI输出）
	Decisions    []Decision `json:"decisions"`       // 具体决策列表
	Timestamp    time.Time  `json:"timestamp"`
	FinishReason string     `json:"finish_reason"`   // API返回的完成原因：stop（正常结束）、length（达到token限制）
}

// GetFullDecision 获取AI的完整交易决策（批量分析所有币种和持仓）
func GetFullDecision(ctx *Context, mcpClient *mcp.Client) (*FullDecision, error) {
	return GetFullDecisionWithCustomPrompt(ctx, mcpClient, "", false, "")
}

// GetFullDecisionWithCustomPrompt 获取AI的完整交易决策（支持自定义prompt和模板选择）
func GetFullDecisionWithCustomPrompt(ctx *Context, mcpClient *mcp.Client, customPrompt string, overrideBase bool, templateName string) (*FullDecision, error) {
	// 1. 为所有币种获取市场数据
	if err := fetchMarketDataForContext(ctx); err != nil {
		return nil, fmt.Errorf("获取市场数据失败: %w", err)
	}

	// 2. 构建 System Prompt（固定规则）和 User Prompt（动态数据）
	systemPrompt := buildSystemPromptWithCustom(ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, customPrompt, overrideBase, templateName)
	userPrompt := buildUserPrompt(ctx)

	// 3. 调用AI API（使用 system + user prompt）
	aiResponse, finishReason, err := mcpClient.CallWithMessages(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("调用AI API失败: %w", err)
	}

	// 4. 解析AI响应
	decision, err := parseFullDecisionResponse(aiResponse, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage)
	if err != nil {
		return decision, fmt.Errorf("解析AI响应失败: %w", err)
	}

	decision.Timestamp = time.Now()
	decision.SystemPrompt = systemPrompt // 保存系统prompt
	decision.UserPrompt = userPrompt     // 保存输入prompt
	decision.FinishReason = finishReason // 保存finish_reason，用于判断是否因token限制被截断
	return decision, nil
}

// fetchMarketDataForContext 为上下文中的所有币种获取市场数据和OI数据
func fetchMarketDataForContext(ctx *Context) error {
	ctx.MarketDataMap = make(map[string]*market.Data)
	ctx.OITopDataMap = make(map[string]*OITopData)
	ctx.PredictionMap = make(map[string]*PredictionData)

	// 收集所有需要获取数据的币种
	symbolSet := make(map[string]bool)

	// 1. 优先获取持仓币种的数据（这是必须的）
	for _, pos := range ctx.Positions {
		symbolSet[pos.Symbol] = true
	}

	// 2. 候选币种数量根据账户状态动态调整
	maxCandidates := calculateMaxCandidates(ctx)
	for i, coin := range ctx.CandidateCoins {
		if i >= maxCandidates {
			break
		}
		symbolSet[coin.Symbol] = true
	}

	// 并发获取市场数据
	// 持仓币种集合（用于判断是否跳过OI检查）
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}

	for symbol := range symbolSet {
		data, err := market.Get(symbol)
		if err != nil {
			// 单个币种失败不影响整体，只记录错误
			continue
		}

		// ⚠️ 流动性过滤：持仓价值低于15M USD的币种不做（多空都不做）
		// 持仓价值 = 持仓量 × 当前价格
		// 但现有持仓必须保留（需要决策是否平仓）
		isExistingPosition := positionSymbols[symbol]
		if !isExistingPosition && data.OpenInterest != nil && data.CurrentPrice > 0 {
			// 计算持仓价值（USD）= 持仓量 × 当前价格
			oiValue := data.OpenInterest.Latest * data.CurrentPrice
			oiValueInMillions := oiValue / 1_000_000 // 转换为百万美元单位
			if oiValueInMillions < 15 {
				log.Printf("⚠️  %s 持仓价值过低(%.2fM USD < 15M)，跳过此币种 [持仓量:%.0f × 价格:%.4f]",
					symbol, oiValueInMillions, data.OpenInterest.Latest, data.CurrentPrice)
				continue
			}
		}

		ctx.MarketDataMap[symbol] = data
	}

	// 加载OI Top数据（不影响主流程）
	oiPositions, err := pool.GetOITopPositions()
	if err == nil {
		for _, pos := range oiPositions {
			// 标准化符号匹配
			symbol := pos.Symbol
			ctx.OITopDataMap[symbol] = &OITopData{
				Rank:              pos.Rank,
				OIDeltaPercent:    pos.OIDeltaPercent,
				OIDeltaValue:      pos.OIDeltaValue,
				PriceDeltaPercent: pos.PriceDeltaPercent,
				NetLong:           pos.NetLong,
				NetShort:          pos.NetShort,
			}
		}
	}

	// 获取krnos模型预测数据（为每个有市场数据的币种）
	// 注意：即使预测失败也不影响主系统运行，只记录日志
	for symbol, data := range ctx.MarketDataMap {
		// 使用defer recover确保单个币种预测失败不影响其他币种
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("⚠️  %s 获取预测数据时发生panic（已恢复，不影响主系统）: %v", symbol, r)
				}
			}()

			// 判断当前趋势（简化版，实际应该从技术指标判断）
			currentTrend := "sideways"
			if data.PriceChange1h > 0.5 {
				currentTrend = "up"
			} else if data.PriceChange1h < -0.5 {
				currentTrend = "down"
			}

		// 获取历史数据（从market包获取，标准450步）
		// 注意：如果获取失败，会使用缓存的预测结果（如果存在）
		var priceHistory, volumeHistory []float64
		historyClient := market.NewHistoryClient()
		klines, err := historyClient.GetLatestKlinesForPrediction(symbol, "3m", 450)
		if err == nil && len(klines) >= 100 {
			// 成功获取历史数据，提取价格和成交量
			priceHistory = make([]float64, len(klines))
			volumeHistory = make([]float64, len(klines))
			for i, kline := range klines {
				priceHistory[i] = kline.Close
				volumeHistory[i] = kline.Volume
			}
		} else {
			// 获取历史数据失败，记录日志但不影响主系统
			if err != nil {
				log.Printf("⚠️  %s 获取历史数据失败（将使用缓存预测）: %v", symbol, err)
			} else {
				log.Printf("⚠️  %s 历史数据不足（只有%d条，需要至少100条），将使用缓存预测", symbol, len(klines))
			}
			// priceHistory 和 volumeHistory 保持为 nil，GetPredictionForSymbol 会使用缓存
		}
		
		// 尝试获取预测数据（容错模式：失败不影响主系统）
		// 注意：GetPredictionForSymbol已经实现了容错，返回nil时不影响主系统
		// 如果历史数据为nil，GetPredictionForSymbol会尝试使用缓存的预测结果
		prediction, err := predictor.GetPredictionForSymbol(
			symbol,
			data.CurrentPrice,
			currentTrend,
			priceHistory, // 传入真实历史数据（如果获取成功）
			volumeHistory, // 传入真实历史数据（如果获取成功）
		)
			
			// 如果预测成功且有结果，存储到PredictionMap
			if err == nil && prediction != nil {
				ctx.PredictionMap[symbol] = prediction
			}
			// 如果预测失败，只记录日志，不影响主系统
			// （GetPredictionForSymbol内部已经记录了详细日志）
		}()
	}

	return nil
}

// calculateMaxCandidates 根据账户状态计算需要分析的候选币种数量
func calculateMaxCandidates(ctx *Context) int {
	// 限制候选币种数量以降低token使用量
	// 最多分析10个候选币种（从全部币种减少到10个）
	maxCandidates := 10
	if len(ctx.CandidateCoins) < maxCandidates {
		return len(ctx.CandidateCoins)
	}
	return maxCandidates
}

// buildSystemPromptWithCustom 构建包含自定义内容的 System Prompt
func buildSystemPromptWithCustom(accountEquity float64, btcEthLeverage, altcoinLeverage int, customPrompt string, overrideBase bool, templateName string) string {
	// 如果覆盖基础prompt且有自定义prompt，只使用自定义prompt
	if overrideBase && customPrompt != "" {
		return customPrompt
	}

	// 获取基础prompt（使用指定的模板）
	basePrompt := buildSystemPrompt(accountEquity, btcEthLeverage, altcoinLeverage, templateName)

	// 如果没有自定义prompt，直接返回基础prompt
	if customPrompt == "" {
		return basePrompt
	}

	// 添加自定义prompt部分到基础prompt
	var sb strings.Builder
	sb.WriteString(basePrompt)
	sb.WriteString("\n\n")
	sb.WriteString("# 📌 个性化交易策略\n\n")
	sb.WriteString(customPrompt)
	sb.WriteString("\n\n")
	sb.WriteString("注意: 以上个性化策略是对基础规则的补充，不能违背基础风险控制原则。\n")

	return sb.String()
}

// buildSystemPrompt 构建 System Prompt（使用模板+动态部分）
func buildSystemPrompt(accountEquity float64, btcEthLeverage, altcoinLeverage int, templateName string) string {
	var sb strings.Builder
	var sectionLengths = make(map[string]int) // 记录各部分长度

	// 1. 加载提示词模板（核心交易策略部分）
	startLen := sb.Len()
	if templateName == "" {
		templateName = "default" // 默认使用 default 模板
	}

	template, err := GetPromptTemplate(templateName)
	if err != nil {
		// 如果模板不存在，记录错误并使用 default
		log.Printf("⚠️  提示词模板 '%s' 不存在，使用 default: %v", templateName, err)
		template, err = GetPromptTemplate("default")
		if err != nil {
			// 如果连 default 都不存在，使用内置的简化版本
			log.Printf("❌ 无法加载任何提示词模板，使用内置简化版本")
			sb.WriteString("你是专业的加密货币交易AI。请根据市场数据做出交易决策。\n\n")
		} else {
			sb.WriteString(template.Content)
			sb.WriteString("\n\n")
		}
	} else {
		sb.WriteString(template.Content)
		sb.WriteString("\n\n")
	}
	sectionLengths[fmt.Sprintf("提示词模板(%s)", templateName)] = sb.Len() - startLen

	// 2. 硬约束（风险控制）- 动态生成
	startLen = sb.Len()
	sb.WriteString("# 硬约束（风险控制）\n\n")
	sb.WriteString("1. 风险回报比: 必须 ≥ 1:3（冒1%风险，赚3%+收益）\n")
	sb.WriteString(fmt.Sprintf("2. **杠杆倍数配置（重要，必须严格遵守）**：\n"))
	sb.WriteString(fmt.Sprintf("   - **BTC/ETH杠杆倍数**: %dx（必须使用此配置的杠杆，不能使用其他值）\n", btcEthLeverage))
	sb.WriteString(fmt.Sprintf("   - **山寨币杠杆倍数**: %dx（必须使用此配置的杠杆，不能使用其他值）\n", altcoinLeverage))
	sb.WriteString("   - **⚠️ 关键**：开仓时必须根据币种使用对应的配置杠杆倍数，不能随意更改\n")
	sb.WriteString(fmt.Sprintf("   - **BTCUSDT和ETHUSDT必须使用%d倍杠杆**，其他币种必须使用%d倍杠杆\n", btcEthLeverage, altcoinLeverage))
	sb.WriteString("   - **不允许使用其他杠杆倍数**，必须严格按照配置使用\n\n")
	sb.WriteString(fmt.Sprintf("3. 仓位控制（基于当前余额百分比，必须使用高仓位）:\n"))
	sb.WriteString(fmt.Sprintf("   - 当前账户总余额: %.2f USDT\n", accountEquity))
	sb.WriteString("   - **⚠️ 关键：开仓前必须检查可用余额**\n")
	sb.WriteString("   - **可用余额检查（硬约束）**：在计算 position_size_usd 之前，必须检查可用余额是否足够\n")
	sb.WriteString("   - **计算所需保证金**：所需保证金 = position_size_usd / 杠杆倍数 × 1.1（含10%缓冲）\n")
	sb.WriteString("   - **如果可用余额 < 所需保证金**：\n")
	sb.WriteString("     - 如果信心度 ≥ 90，可以尝试平掉表现较差的仓位释放保证金\n")
	sb.WriteString("     - 否则，必须 wait 或调整仓位大小，确保所需保证金 ≤ 可用余额\n")
	sb.WriteString("   - **每笔操作使用仓位 ≥ 30%**（硬约束，必须遵守，但前提是可用余额足够）\n")
	sb.WriteString("   - **⚠️ 关键：机会判断严格，如果真的有机会能开单，应该合理使用高百分比仓位**\n")
	sb.WriteString("   - **标准机会**：使用30-50%的账户总余额作为保证金（最低30%，不要保守）\n")
	sb.WriteString("   - **A级机会**：使用50-70%的账户总余额作为保证金（最低50%，勇敢使用大仓位）\n")
	sb.WriteString("   - **优质机会**（趋势非常明确 + 多周期一致 + 盈亏比≥3:1 + 盈利空间>15%）：可以使用70-90%的账户总余额作为保证金（最低70%，充分利用好机会）\n")
	sb.WriteString("   - **最小订单金额（硬约束）**：订单名义价值（position_size_usd）必须 ≥ **20 USDT**（币安最小限制）\n")
	sb.WriteString("   - 如果计算出的仓位金额 < 20 USDT，必须至少使用20 USDT（但建议使用更大的百分比）\n")
	sb.WriteString("3. 保证金: 总使用率 ≤ 90%\n")
	sb.WriteString("4. 在更好的机会时，要勇于增加仓位，不要被仓位限制束缚，争取更大收益\n\n")
	sectionLengths["硬约束(风险控制)"] = sb.Len() - startLen

	// 3. 输出格式 - 动态生成
	startLen = sb.Len()
	sb.WriteString("#输出格式\n\n")
	sb.WriteString("第一步: 思维链（纯文本）\n")
	sb.WriteString("简洁分析你的思考过程\n\n")
	sb.WriteString("第二步: JSON决策数组\n\n")
	sb.WriteString("```json\n[\n")
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"position_size_usd\": %.0f, \"stop_loss\": 97000, \"take_profit\": 91000, \"confidence\": 85, \"risk_usd\": 300, \"reasoning\": \"下跌趋势+MACD死叉\"}\n", btcEthLeverage, accountEquity*float64(btcEthLeverage)*0.5))
	sb.WriteString("]\n```\n\n")
	sb.WriteString("**如果没有决策，输出：**\n")
	sb.WriteString("```json\n[]\n```\n\n")
	sb.WriteString("**⚠️ 再次强调：必须输出完整的JSON数组，即使为空也要输出 []，不能省略！**\n\n")
	sb.WriteString("**示例（有决策）：**\n")
	sb.WriteString("```json\n[\n")
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"position_size_usd\": %.0f, \"stop_loss\": 97000, \"take_profit\": 91000, \"confidence\": 85, \"risk_usd\": 300, \"reasoning\": \"下跌趋势+MACD死叉\"},\n", btcEthLeverage, accountEquity*float64(btcEthLeverage)*0.5))
	sb.WriteString("  {\"symbol\": \"ETHUSDT\", \"action\": \"close_long\", \"reasoning\": \"止盈离场\"},\n")
	sb.WriteString("  {\"symbol\": \"HYPEUSDT\", \"action\": \"hold\", \"stop_loss\": 33.20, \"take_profit\": 35.50, \"reasoning\": \"继续持仓，上移止损保护利润，调整止盈目标\"}\n")
	sb.WriteString("]\n```\n\n")
	sb.WriteString("字段说明:\n")
	sb.WriteString("- `action`: open_long | open_short | close_long | close_short | hold | wait\n")
	sb.WriteString("- `confidence`: 0-100（开仓建议≥75）\n")
	sb.WriteString("- **开仓时必填**: leverage, position_size_usd, stop_loss, take_profit, confidence, risk_usd, reasoning\n")
	sb.WriteString(fmt.Sprintf("- **leverage字段**：必须使用配置的杠杆倍数（BTC/ETH使用%d倍，山寨币使用%d倍），不能使用其他值\n", btcEthLeverage, altcoinLeverage))
	sb.WriteString("- **hold时可选**: stop_loss, take_profit（如果提供，将更新持仓的止损止盈价格）\n")
	sb.WriteString("  - 如果市场情况变化，可以在hold时调整止损止盈以优化风险控制\n")
	sb.WriteString("  - 例如：价格上涨后，可以上移止损保护利润；或根据新的技术分析调整止盈目标\n")
	sb.WriteString("  - 如果不需要调整止损止盈，可以不提供这两个字段\n\n")
	sectionLengths["输出格式"] = sb.Len() - startLen

	// 输出各部分统计
	totalSystemPromptLen := sb.Len()
	log.Printf("📊 [System Prompt] 各部分长度统计:")
	for section, length := range sectionLengths {
		if length > 0 {
			percentage := float64(length) / float64(totalSystemPromptLen) * 100
			estimatedTokens := int(float64(length) * 1.5)
			log.Printf("   %s: %d 字符 (%.1f%%, 估算 %d tokens)", section, length, percentage, estimatedTokens)
		}
	}
	log.Printf("   System Prompt 总计: %d 字符 (估算 %d tokens)", totalSystemPromptLen, int(float64(totalSystemPromptLen)*1.5))

	return sb.String()
}

// buildUserPrompt 构建 User Prompt（动态数据）
func buildUserPrompt(ctx *Context) string {
	var sb strings.Builder
	var sectionLengths = make(map[string]int) // 记录各部分长度

	// 系统状态
	// 优先使用HourlyTradeCount，如果没有则使用TodayTradeCount（向后兼容）
	tradeCount := ctx.HourlyTradeCount
	if tradeCount == 0 {
		tradeCount = ctx.TodayTradeCount
	}
	
	startLen := sb.Len()
	// 简化系统状态显示
	if ctx.HoursSinceLastTrade >= 0 {
		sb.WriteString(fmt.Sprintf("时间:%s 周期:#%d 运行:%dm 交易:%d次 距上次:%.1fh\n\n",
			ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes, tradeCount, ctx.HoursSinceLastTrade))
	} else {
		sb.WriteString(fmt.Sprintf("时间:%s 周期:#%d 运行:%dm 交易:%d次\n\n",
			ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes, tradeCount))
	}
	sectionLengths["系统状态"] = sb.Len() - startLen

	// BTC 市场（简化）
	startLen = sb.Len()
	if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
		sb.WriteString(fmt.Sprintf("BTC:%.2f 1h:%+.2f%% 4h:%+.2f%% MACD:%.4f RSI:%.2f\n\n",
			btcData.CurrentPrice, btcData.PriceChange1h, btcData.PriceChange4h,
			btcData.CurrentMACD, btcData.CurrentRSI7))
	}
	sectionLengths["BTC市场"] = sb.Len() - startLen

	// 账户（简化显示）
	startLen = sb.Len()
	usedMargin := ctx.Account.TotalEquity - ctx.Account.AvailableBalance
	sb.WriteString(fmt.Sprintf("账户:净值%.2f 可用:%.2f(%.1f%%) 已用:%.2f(%.1f%%) 盈亏:%.2f%% 持仓:%d\n\n",
		ctx.Account.TotalEquity,
		ctx.Account.AvailableBalance,
		(ctx.Account.AvailableBalance/ctx.Account.TotalEquity)*100,
		usedMargin,
		(usedMargin/ctx.Account.TotalEquity)*100,
		ctx.Account.TotalPnLPct,
		ctx.Account.PositionCount))
	sectionLengths["账户信息"] = sb.Len() - startLen

	// 自动检测到的平仓信息（止损/止盈触发）
	if len(ctx.AutoClosedPositions) > 0 {
		sb.WriteString("## ⚠️ 自动检测到的平仓（止损/止盈触发）\n")
		sb.WriteString("**注意**：以下持仓已被自动平仓（不是AI决策），AI无需对这些持仓做出决策：\n\n")
		for _, closed := range ctx.AutoClosedPositions {
			sb.WriteString(fmt.Sprintf("- **%s %s**：入场价 %.4f，平仓价 %.4f，数量 %.4f，杠杆 %dx，原因：%s\n",
				closed.Symbol, strings.ToUpper(closed.Side), closed.EntryPrice, closed.ClosePrice,
				closed.Quantity, closed.Leverage, closed.Reason))
		}
		sb.WriteString("\n")
	}

	// 持仓（完整市场数据）
	startLen = sb.Len()
	if len(ctx.Positions) > 0 {
		sb.WriteString("## 当前持仓\n")
		for i, pos := range ctx.Positions {
			// 简化持仓信息显示
			sb.WriteString(fmt.Sprintf("%d. %s %s 入场:%.4f 当前:%.4f 盈亏:%.2f%% 杠杆:%dx 强平:%.4f\n",
				i+1, pos.Symbol, strings.ToUpper(pos.Side),
				pos.EntryPrice, pos.MarkPrice, pos.UnrealizedPnLPct,
				pos.Leverage, pos.LiquidationPrice))

			// 使用FormatMarketData输出完整市场数据
			if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
				sb.WriteString(market.Format(marketData))
				sb.WriteString("\n")
			}
			
			// 显示krnos模型预测数据（包含预测数据点，用于AI判断拟合程度）
			if prediction, ok := ctx.PredictionMap[pos.Symbol]; ok {
				// 保存简单数据到本地（不计算详细拟合度）
				if marketData, ok2 := ctx.MarketDataMap[pos.Symbol]; ok2 {
					savePredictionDataSimple(pos.Symbol, marketData.CurrentPrice, prediction)
					
					// 输出预测信息给AI
					sb.WriteString("**krnos模型预测**:\n")
					sb.WriteString(fmt.Sprintf("- **预测趋势**: %s\n", prediction.Trend))
					if len(prediction.PriceRange) >= 2 {
						sb.WriteString(fmt.Sprintf("- **价格范围**: [%.2f, %.2f]\n", prediction.PriceRange[0], prediction.PriceRange[1]))
					}
					
					// 输出预测数据点（用于判断拟合程度）
					if len(prediction.MeanPrediction) > 0 {
						outputPredictionDataPoints(&sb, prediction, marketData.CurrentPrice)
					}
					sb.WriteString("\n")
				}
			}
		}
	}
	sb.WriteString("\n")
	sb.WriteString("\n")
	sectionLengths[fmt.Sprintf("持仓信息(%d个)", len(ctx.Positions))] = sb.Len() - startLen

	// 候选币种（完整市场数据）
	// 只显示用户选择的候选币种（已经在fetchMarketDataForContext中过滤）
	candidateSymbols := make(map[string]bool)
	maxCandidates := calculateMaxCandidates(ctx)
	for i, coin := range ctx.CandidateCoins {
		if i >= maxCandidates {
			break
		}
		candidateSymbols[coin.Symbol] = true
	}
	
	// 过滤掉已经是持仓的币种（持仓已经在上面显示了）
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}
	
	startLen = sb.Len()
	if len(candidateSymbols) > 0 {
		sb.WriteString("## 候选币种（用户选择的币种）\n")
		candidateIndex := 1
		candidateCount := 0
		for symbol := range candidateSymbols {
			// 跳过已经是持仓的币种
			if positionSymbols[symbol] {
				continue
			}
			
			// 只显示有市场数据的币种
			if marketData, ok := ctx.MarketDataMap[symbol]; ok {
				sb.WriteString(fmt.Sprintf("%d. %s\n", candidateIndex, symbol))
				sb.WriteString(market.Format(marketData))
				sb.WriteString("\n")
				
				// 显示krnos模型预测数据（包含预测数据点）
				if prediction, ok := ctx.PredictionMap[symbol]; ok {
					// 保存简单数据到本地
					if marketData, ok2 := ctx.MarketDataMap[symbol]; ok2 {
						savePredictionDataSimple(symbol, marketData.CurrentPrice, prediction)
						
						// 输出预测信息给AI（简化格式）
						sb.WriteString("**krnos预测**: ")
						sb.WriteString(fmt.Sprintf("趋势=%s", prediction.Trend))
						if len(prediction.PriceRange) >= 2 {
							sb.WriteString(fmt.Sprintf(" 范围=[%.2f,%.2f]", prediction.PriceRange[0], prediction.PriceRange[1]))
						}
						sb.WriteString("\n")
						
						// 输出预测数据点（用于判断拟合程度）
						if len(prediction.MeanPrediction) > 0 {
							outputPredictionDataPoints(&sb, prediction, marketData.CurrentPrice)
						}
					}
					sb.WriteString("\n")
				}
				
				sb.WriteString("\n")
				candidateIndex++
				candidateCount++
			}
		}
		sb.WriteString("\n")
		sectionLengths[fmt.Sprintf("候选币种(%d个)", candidateCount)] = sb.Len() - startLen
	}

	// 历史表现分析（包括夏普比率和已完成交易详情）
	startLen = sb.Len()
	if ctx.Performance != nil {
		// 提取PerformanceAnalysis数据
		type PerformanceData struct {
			SharpeRatio  float64 `json:"sharpe_ratio"`
			RecentTrades []struct {
				Symbol        string    `json:"symbol"`
				Side          string    `json:"side"`
				OpenPrice     float64   `json:"open_price"`
				ClosePrice    float64   `json:"close_price"`
				PnL           float64   `json:"pn_l"`           // 毛盈亏
				PnLPct        float64   `json:"pn_l_pct"`      // 毛盈亏百分比
				Fee           float64   `json:"fee"`           // 手续费
				NetPnL        float64   `json:"net_pn_l"`      // 净盈亏（扣除手续费）
				NetPnLPct     float64   `json:"net_pn_l_pct"` // 净盈亏百分比
				Duration      string    `json:"duration"`
				PositionValue float64   `json:"position_value"`
				MarginUsed    float64   `json:"margin_used"`
				Leverage      int       `json:"leverage"`
				OpenTime      time.Time `json:"open_time"`
				CloseTime     time.Time `json:"close_time"`
				WasStopLoss   bool     `json:"was_stop_loss"`
			} `json:"recent_trades"`
			TotalTrades   int     `json:"total_trades"`
			WinningTrades  int     `json:"winning_trades"`
			LosingTrades   int     `json:"losing_trades"`
			WinRate        float64 `json:"win_rate"`
			AvgWin         float64 `json:"avg_win"`
			AvgLoss        float64 `json:"avg_loss"`
			ProfitFactor   float64 `json:"profit_factor"`
		}
		var perfData PerformanceData
		if jsonData, err := json.Marshal(ctx.Performance); err == nil {
			if err := json.Unmarshal(jsonData, &perfData); err == nil {
				// 显示夏普比率
				// 简化历史表现统计
				sb.WriteString("## 📊 历史表现\n")
				sb.WriteString(fmt.Sprintf("夏普:%.2f 交易:%d 胜率:%.1f%% 盈亏比:%.2f 均盈:%.2f 均亏:%.2f\n\n",
					perfData.SharpeRatio, perfData.TotalTrades, perfData.WinRate, perfData.ProfitFactor,
					perfData.AvgWin, perfData.AvgLoss))

				// 显示最近完成的交易详情
				// 减少到2笔以降低token使用量
				if len(perfData.RecentTrades) > 0 {
					sb.WriteString("## 📈 最近交易（2笔）\n")
					sb.WriteString("|币种|方向|开仓|平仓|净盈亏|净%|杠杆|\n")
					sb.WriteString("|---|---|---|---|---|---|---|\n")
					
					// 只显示最近2笔交易以降低token使用量
					maxTrades := 2
					if len(perfData.RecentTrades) > maxTrades {
						perfData.RecentTrades = perfData.RecentTrades[:maxTrades]
					}
					
					for _, trade := range perfData.RecentTrades {
						// 盈亏标记（基于净盈亏）
						pnlSymbol := "✅"
						if trade.NetPnL < 0 {
							pnlSymbol = "❌"
						} else if trade.NetPnL == 0 {
							pnlSymbol = "➖"
						}
						
						// 进一步简化显示
						sb.WriteString(fmt.Sprintf("|%s|%s|%.2f|%.2f|%s%.2f|%.1f%%|%dx|\n",
							trade.Symbol,
							strings.ToUpper(trade.Side),
							trade.OpenPrice,
							trade.ClosePrice,
							pnlSymbol,
							trade.NetPnL,
							trade.NetPnLPct,
							trade.Leverage))
					}
					sb.WriteString("\n")
					sb.WriteString("**交易详情说明**:\n")
					sb.WriteString("- **净盈亏**: 扣除手续费后的实际盈亏（主要指标）\n")
					sb.WriteString("- **毛盈亏**: 未扣除手续费的盈亏（括号内显示）\n")
					sb.WriteString("- **净盈亏**: 扣除手续费后的实际盈亏（总盈亏 - 手续费）\n")
					sb.WriteString("- **盈亏%**: 相对保证金的盈亏百分比\n")
					sb.WriteString("- **手续费**: 开仓0.04% + 平仓0.04% = 0.08% × 仓位价值（开仓价 × 数量）\n")
					sb.WriteString("- **持仓时长**: 从开仓到平仓的时间\n")
					sb.WriteString("- **止损**: 是否因触发止损而平仓\n")
					sb.WriteString("- ✅ 盈利 | ❌ 亏损 | ➖ 持平\n\n")
				}
			}
		}
	}
	sectionLengths["历史表现"] = sb.Len() - startLen

	startLen = sb.Len()
	sb.WriteString("---\n\n")
	sb.WriteString("现在请分析并输出决策（思维链 + JSON）\n")
	sectionLengths["结尾提示"] = sb.Len() - startLen

	// 输出各部分统计
	totalUserPromptLen := sb.Len()
	log.Printf("📊 [User Prompt] 各部分长度统计:")
	for section, length := range sectionLengths {
		if length > 0 {
			percentage := float64(length) / float64(totalUserPromptLen) * 100
			estimatedTokens := int(float64(length) * 1.5)
			log.Printf("   %s: %d 字符 (%.1f%%, 估算 %d tokens)", section, length, percentage, estimatedTokens)
		}
	}
	log.Printf("   User Prompt 总计: %d 字符 (估算 %d tokens)", totalUserPromptLen, int(float64(totalUserPromptLen)*1.5))

	return sb.String()
}


// parseFullDecisionResponse 解析AI的完整决策响应
func parseFullDecisionResponse(aiResponse string, accountEquity float64, btcEthLeverage, altcoinLeverage int) (*FullDecision, error) {
	// 1. 提取思维链
	cotTrace := extractCoTTrace(aiResponse)

	// 2. 提取JSON决策列表
	decisions, err := extractDecisions(aiResponse)
	if err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: []Decision{},
		}, fmt.Errorf("提取决策失败: %w", err)
	}

	// 3. 验证决策
	if err := validateDecisions(decisions, accountEquity, btcEthLeverage, altcoinLeverage); err != nil {
		return &FullDecision{
			CoTTrace:  cotTrace,
			Decisions: decisions,
		}, fmt.Errorf("决策验证失败: %w", err)
	}

	return &FullDecision{
		CoTTrace:  cotTrace,
		Decisions: decisions,
	}, nil
}

// extractCoTTrace 提取思维链分析（修复版本：智能查找JSON开始位置，避免思维链中的[字符被误判）
func extractCoTTrace(response string) string {
	// 方法1: 查找 ```json 代码块
	jsonBlockStart := strings.Index(response, "```json")
	if jsonBlockStart != -1 {
		// 思维链是代码块之前的内容
		return strings.TrimSpace(response[:jsonBlockStart])
	}

	// 方法2: 查找 ``` 代码块（可能是markdown格式）
	codeBlockStart := strings.Index(response, "```")
	if codeBlockStart != -1 {
		// 检查是否是JSON代码块（跳过 ``` 和可能的语言标识符）
		contentStart := codeBlockStart + 3
		// 跳过可能的语言标识符和换行
		for contentStart < len(response) && (response[contentStart] == ' ' || response[contentStart] == '\n' ||
			(response[contentStart] >= 'a' && response[contentStart] <= 'z')) {
			if response[contentStart] == '\n' {
				contentStart++
				break
			}
			contentStart++
		}
		// 检查代码块内容是否以 [ 或 { 开始（JSON格式）
		if contentStart < len(response) && (response[contentStart] == '[' || response[contentStart] == '{') {
			// 这是JSON代码块，思维链是代码块之前的内容
			return strings.TrimSpace(response[:codeBlockStart])
		}
	}

	// 方法3: 智能查找JSON数组开始位置（改进版：排除思维链中的[字符）
	// 查找 [ 后面跟着换行或空格，然后是 { 或 " 的结构（JSON数组格式）
	// 但需要排除思维链中的[字符（如"价格范围: ["、"预测曲线: ["等）
	jsonStart := -1
	for i := 0; i < len(response)-10; i++ {
		if response[i] == '[' {
			// 检查前面是否有上下文关键词（排除思维链中的[字符）
			// 查找[前面的内容，检查是否包含关键词
			contextStart := i - 50
			if contextStart < 0 {
				contextStart = 0
			}
			context := response[contextStart:i]
			
			// 如果[前面包含这些关键词，说明这是思维链中的[字符，不是JSON开始
			excludeKeywords := []string{
				"价格范围:",
				"预测曲线:",
				"置信区间:",
				"价格范围",
				"预测曲线",
				"置信区间",
				"范围:",
				"曲线:",
				"区间:",
			}
			
			isExcluded := false
			for _, keyword := range excludeKeywords {
				if strings.Contains(context, keyword) {
					isExcluded = true
					break
				}
			}
			
			if isExcluded {
				continue // 跳过这个[字符，继续查找
			}
			
			// 检查后面是否是JSON格式
			j := i + 1
			// 跳过空白字符
			for j < len(response) && (response[j] == ' ' || response[j] == '\n' || response[j] == '\t' || response[j] == '\r') {
				j++
			}
			// 检查是否是JSON对象或字符串的开始
			if j < len(response) && (response[j] == '{' || response[j] == '"') {
				jsonStart = i
				break
			}
		}
	}

	if jsonStart > 0 {
		// 思维链是JSON数组之前的内容
		return strings.TrimSpace(response[:jsonStart])
	}

	// 如果找不到JSON，整个响应都是思维链
	return strings.TrimSpace(response)
}

// extractDecisions 提取JSON决策列表（容错模式）
func extractDecisions(response string) ([]Decision, error) {
	// 尝试多种方式提取JSON
	
	// 方法1: 查找 ```json 代码块
	jsonBlockStart := strings.Index(response, "```json")
	if jsonBlockStart != -1 {
		// 找到代码块开始，查找代码块结束
		jsonBlockEnd := strings.Index(response[jsonBlockStart+7:], "```")
		if jsonBlockEnd != -1 {
			jsonContent := strings.TrimSpace(response[jsonBlockStart+7 : jsonBlockStart+7+jsonBlockEnd])
			decisions, err := parseJSONContent(jsonContent)
			if err == nil {
				return decisions, nil
			}
			// 如果解析失败，继续尝试其他方法
		}
	}
	
	// 方法2: 查找 ``` 代码块（可能是markdown格式）
	codeBlockStart := strings.Index(response, "```")
	if codeBlockStart != -1 {
		// 跳过开头的 ```
		contentStart := codeBlockStart + 3
		// 跳过可能的语言标识符（如 json）
		for contentStart < len(response) && (response[contentStart] == ' ' || response[contentStart] == '\n' || 
			(response[contentStart] >= 'a' && response[contentStart] <= 'z')) {
			if response[contentStart] == '\n' {
				contentStart++
				break
			}
			contentStart++
		}
		// 查找代码块结束
		codeBlockEnd := strings.Index(response[contentStart:], "```")
		if codeBlockEnd != -1 {
			jsonContent := strings.TrimSpace(response[contentStart : contentStart+codeBlockEnd])
			decisions, err := parseJSONContent(jsonContent)
			if err == nil {
				return decisions, nil
			}
		}
	}
	
	// 方法3: 直接查找JSON数组
	arrayStart := strings.Index(response, "[")
	if arrayStart != -1 {
	// 从 [ 开始，匹配括号找到对应的 ]
	arrayEnd := findMatchingBracket(response, arrayStart)
		if arrayEnd != -1 {
			jsonContent := strings.TrimSpace(response[arrayStart : arrayEnd+1])
			decisions, err := parseJSONContent(jsonContent)
			if err == nil {
				return decisions, nil
			}
		} else {
			// 如果找不到匹配的 ]，尝试从 [ 开始到文本结束，然后尝试修复
			// 查找最后一个可能的 ]
			lastBracket := strings.LastIndex(response[arrayStart:], "]")
			if lastBracket != -1 {
				jsonContent := strings.TrimSpace(response[arrayStart : arrayStart+lastBracket+1])
				// 尝试修复不完整的JSON（添加缺失的 ]）
				if !strings.HasSuffix(jsonContent, "]") {
					// 计算需要添加的 ]
					depth := 0
					for _, c := range jsonContent {
						if c == '[' {
							depth++
						} else if c == ']' {
							depth--
						}
					}
					if depth > 0 {
						jsonContent += strings.Repeat("]", depth)
					}
				}
				decisions, err := parseJSONContent(jsonContent)
				if err == nil {
					log.Printf("⚠️  JSON数组不完整，已尝试修复并解析成功")
					return decisions, nil
				}
			}
		}
	}
	
	// 方法4: 如果所有方法都失败，返回空数组（容错模式）
	log.Printf("⚠️  无法提取JSON决策，返回空数组（容错模式）")
	log.Printf("   响应预览: %s", truncateString(response, 500))
	return []Decision{}, nil
}

// parseJSONContent 解析JSON内容（带容错处理）
func parseJSONContent(jsonContent string) ([]Decision, error) {
	// 🔧 修复常见的JSON格式错误
	jsonContent = fixMissingQuotes(jsonContent)

	// 尝试解析JSON
	var decisions []Decision
	if err := json.Unmarshal([]byte(jsonContent), &decisions); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}

	return decisions, nil
}

// truncateString 截断字符串用于日志显示
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// fixMissingQuotes 替换中文引号为英文引号（避免输入法自动转换）
func fixMissingQuotes(jsonStr string) string {
	jsonStr = strings.ReplaceAll(jsonStr, "\u201c", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u201d", "\"") // "
	jsonStr = strings.ReplaceAll(jsonStr, "\u2018", "'")  // '
	jsonStr = strings.ReplaceAll(jsonStr, "\u2019", "'")  // '
	return jsonStr
}

// validateDecisions 验证所有决策（需要账户信息和杠杆配置）
func validateDecisions(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int) error {
	for i, decision := range decisions {
		if err := validateDecision(&decision, accountEquity, btcEthLeverage, altcoinLeverage); err != nil {
			return fmt.Errorf("决策 #%d 验证失败: %w", i+1, err)
		}
	}
	return nil
}

// findMatchingBracket 查找匹配的右括号
func findMatchingBracket(s string, start int) int {
	if start >= len(s) || s[start] != '[' {
		return -1
	}

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

// validateDecision 验证单个决策的有效性
func validateDecision(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int) error {
	// 验证action
	validActions := map[string]bool{
		"open_long":   true,
		"open_short":  true,
		"close_long":  true,
		"close_short": true,
		"hold":        true,
		"wait":        true,
	}

	if !validActions[d.Action] {
		return fmt.Errorf("无效的action: %s", d.Action)
	}

	// 开仓操作必须提供完整参数
	if d.Action == "open_long" || d.Action == "open_short" {
		// 根据币种使用配置的杠杆上限
		maxLeverage := altcoinLeverage // 山寨币使用配置的杠杆
		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			maxLeverage = btcEthLeverage // BTC和ETH使用配置的杠杆
		}

		if d.Leverage <= 0 || d.Leverage > maxLeverage {
			return fmt.Errorf("杠杆必须在1-%d之间（%s，当前配置上限%d倍）: %d", maxLeverage, d.Symbol, maxLeverage, d.Leverage)
		}
		if d.PositionSizeUSD <= 0 {
			return fmt.Errorf("仓位大小必须大于0: %.2f", d.PositionSizeUSD)
		}
		// 验证最小订单金额（币安要求订单名义价值 >= 20 USDT）
		const minOrderNotional = 20.0
		if d.PositionSizeUSD < minOrderNotional {
			return fmt.Errorf("订单名义价值必须 >= %.0f USDT（币安最小限制），实际: %.2f USDT。请增加仓位大小或选择其他币种", minOrderNotional, d.PositionSizeUSD)
		}
		if d.StopLoss <= 0 || d.TakeProfit <= 0 {
			return fmt.Errorf("止损和止盈必须大于0")
		}

		// 验证止损止盈的合理性
		if d.Action == "open_long" {
			if d.StopLoss >= d.TakeProfit {
				return fmt.Errorf("做多时止损价必须小于止盈价")
			}
		} else {
			if d.StopLoss <= d.TakeProfit {
				return fmt.Errorf("做空时止损价必须大于止盈价")
			}
		}

		// 验证风险回报比（必须≥1:3）
		// 计算入场价（假设当前市价）
		var entryPrice float64
		if d.Action == "open_long" {
			// 做多：入场价在止损和止盈之间
			entryPrice = d.StopLoss + (d.TakeProfit-d.StopLoss)*0.2 // 假设在20%位置入场
		} else {
			// 做空：入场价在止损和止盈之间
			entryPrice = d.StopLoss - (d.StopLoss-d.TakeProfit)*0.2 // 假设在20%位置入场
		}

		var riskPercent, rewardPercent, riskRewardRatio float64
		if d.Action == "open_long" {
			riskPercent = (entryPrice - d.StopLoss) / entryPrice * 100
			rewardPercent = (d.TakeProfit - entryPrice) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		} else {
			riskPercent = (d.StopLoss - entryPrice) / entryPrice * 100
			rewardPercent = (entryPrice - d.TakeProfit) / entryPrice * 100
			if riskPercent > 0 {
				riskRewardRatio = rewardPercent / riskPercent
			}
		}

		// 硬约束：风险回报比必须≥3.0
		if riskRewardRatio < 3.0 {
			return fmt.Errorf("风险回报比过低(%.2f:1)，必须≥3.0:1 [风险:%.2f%% 收益:%.2f%%] [止损:%.2f 止盈:%.2f]",
				riskRewardRatio, riskPercent, rewardPercent, d.StopLoss, d.TakeProfit)
		}
	}

	return nil
}

// outputPredictionDataPoints 输出预测数据点到prompt（用于AI判断拟合程度）
func outputPredictionDataPoints(sb *strings.Builder, prediction *PredictionData, currentPrice float64) {
	// 解析预测生成时间
	var predictionTime time.Time
	if prediction.Timestamp != "" {
		timeFormats := []string{
			"2006-01-02 15:04:05",
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02T15:04:05Z07:00",
		}
		for _, format := range timeFormats {
			if t, err := time.Parse(format, prediction.Timestamp); err == nil {
				predictionTime = t
				break
			}
		}
	}
	if predictionTime.IsZero() {
		predictionTime = time.Now()
	}

	// 计算当前时间对应的预测数据点索引
	currentTime := time.Now()
	timeSincePrediction := currentTime.Sub(predictionTime)
	currentIndex := int(timeSincePrediction.Minutes() / 3.0)
	if currentIndex < 0 {
		currentIndex = 0
	}
	if currentIndex >= len(prediction.MeanPrediction) {
		currentIndex = len(prediction.MeanPrediction) - 1
	}

	// 显示已过去的预测数据点（最多10个，用于验证拟合程度）
	if currentIndex > 0 {
		pastCount := currentIndex
		if pastCount > 10 {
			pastCount = 10 // 最多显示10个过去点
		}
		// 计算时间范围
		pastStartIdx := currentIndex - pastCount
		if pastStartIdx < 0 {
			pastStartIdx = 0
		}
		pastStartTime := predictionTime.Add(time.Duration(pastStartIdx+1) * 3 * time.Minute)
		pastEndTime := predictionTime.Add(time.Duration(currentIndex) * 3 * time.Minute)
		
		sb.WriteString(fmt.Sprintf("- 过去预测点（%d个，预测时间 %s-%s）: ", 
			pastCount, 
			pastStartTime.Format("15:04"), 
			pastEndTime.Format("15:04")))
		startIdx := currentIndex - pastCount
		if startIdx < 0 {
			startIdx = 0
		}
		for i := startIdx; i < currentIndex; i++ {
			predictedTime := predictionTime.Add(time.Duration(i+1) * 3 * time.Minute)
			sb.WriteString(fmt.Sprintf("%s:%.2f", predictedTime.Format("15:04"), prediction.MeanPrediction[i]))
			if i < currentIndex-1 {
				sb.WriteString(",")
			}
		}
		sb.WriteString(fmt.Sprintf(" 当前:%.2f\n", currentPrice))
	}

	// 显示未来的预测数据点（最多30个，用于决策，减少以降低token使用）
	futureCount := 30
	if len(prediction.MeanPrediction)-currentIndex < futureCount {
		futureCount = len(prediction.MeanPrediction) - currentIndex
	}
	if futureCount > 0 {
		// 根据价格范围动态确定小数位数
		priceFormat := "%.2f"
		if len(prediction.MeanPrediction) > 0 {
			avgPrice := prediction.MeanPrediction[currentIndex]
			for i := currentIndex + 1; i < currentIndex+futureCount && i < len(prediction.MeanPrediction); i++ {
				avgPrice += prediction.MeanPrediction[i]
			}
			avgPrice /= float64(futureCount)
			if avgPrice < 1.0 {
				priceFormat = "%.6f"
			} else if avgPrice < 10.0 {
				priceFormat = "%.4f"
			}
		}

		// 计算时间范围（从预测生成时间开始，而不是当前时间）
		futureStartTime := predictionTime.Add(time.Duration(currentIndex+1) * 3 * time.Minute)
		futureEndTime := predictionTime.Add(time.Duration(currentIndex+futureCount) * 3 * time.Minute)
		
		// 显示时间范围和预测点
		sb.WriteString(fmt.Sprintf("- 未来预测点（%d个，预测时间 %s-%s）: ", 
			futureCount, 
			futureStartTime.Format("15:04"), 
			futureEndTime.Format("15:04")))
		for i := currentIndex; i < currentIndex+futureCount && i < len(prediction.MeanPrediction); i++ {
			sb.WriteString(fmt.Sprintf(priceFormat, prediction.MeanPrediction[i]))
			if i < currentIndex+futureCount-1 && i < len(prediction.MeanPrediction)-1 {
				sb.WriteString(",")
			}
			// 每15个值换行
			if (i-currentIndex+1)%15 == 0 && i < currentIndex+futureCount-1 {
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}

	// 显示置信区间
	if len(prediction.ConfidenceIntervalLower) >= 2 && len(prediction.ConfidenceIntervalUpper) >= 2 {
		// 计算整个预测区间的置信区间范围
		minLower := prediction.ConfidenceIntervalLower[0]
		maxUpper := prediction.ConfidenceIntervalUpper[0]
		for i := 1; i < len(prediction.ConfidenceIntervalLower); i++ {
			if prediction.ConfidenceIntervalLower[i] < minLower {
				minLower = prediction.ConfidenceIntervalLower[i]
			}
		}
		for i := 1; i < len(prediction.ConfidenceIntervalUpper); i++ {
			if prediction.ConfidenceIntervalUpper[i] > maxUpper {
				maxUpper = prediction.ConfidenceIntervalUpper[i]
			}
		}
		sb.WriteString(fmt.Sprintf("- 95%%置信区间: [%.2f, %.2f]\n", minLower, maxUpper))
	}
}

// savePredictionDataSimple 保存简单预测数据到本地（不计算详细拟合度）
func savePredictionDataSimple(symbol string, currentPrice float64, prediction *PredictionData) {
	if len(prediction.MeanPrediction) == 0 {
		return
	}

	// 解析预测生成时间
	var predictionTime time.Time
	if prediction.Timestamp != "" {
		timeFormats := []string{
			"2006-01-02 15:04:05",
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02T15:04:05Z07:00",
		}
		for _, format := range timeFormats {
			if t, err := time.Parse(format, prediction.Timestamp); err == nil {
				predictionTime = t
				break
			}
		}
	}
	if predictionTime.IsZero() {
		predictionTime = time.Now()
	}

	// 计算当前时间对应的预测数据点索引
	currentTime := time.Now()
	timeSincePrediction := currentTime.Sub(predictionTime)
	currentIndex := int(timeSincePrediction.Minutes() / 3.0)
	if currentIndex < 0 {
		currentIndex = 0
	}
	if currentIndex >= len(prediction.MeanPrediction) {
		currentIndex = len(prediction.MeanPrediction) - 1
	}

	// 保存简单数据（不计算拟合度指标）
	fitData := map[string]interface{}{
		"symbol":          symbol,
		"timestamp":       currentTime.Format(time.RFC3339),
		"current_price":   currentPrice,
		"prediction_time": predictionTime.Format(time.RFC3339),
		"current_index":   currentIndex,
		"trend":           prediction.Trend,
		"trend_strength":  prediction.TrendStrength,
		"price_range":     prediction.PriceRange,
	}

	// 保存已过去的预测点（用于后续分析拟合度）
	if currentIndex > 0 {
		pastPredictions := make([]map[string]interface{}, 0)
		for i := 0; i < currentIndex && i < len(prediction.MeanPrediction); i++ {
			predictedTime := predictionTime.Add(time.Duration(i+1) * 3 * time.Minute)
			pastPredictions = append(pastPredictions, map[string]interface{}{
				"index":        i,
				"time":         predictedTime.Format(time.RFC3339),
				"predicted":    prediction.MeanPrediction[i],
				"lower_bound":  prediction.ConfidenceIntervalLower[i],
				"upper_bound":  prediction.ConfidenceIntervalUpper[i],
			})
		}
		fitData["past_predictions"] = pastPredictions
		fitData["past_points_count"] = len(pastPredictions)
	}

	// 保存未来预测点
	if currentIndex < len(prediction.MeanPrediction) {
		futurePredictions := make([]map[string]interface{}, 0)
		futureCount := len(prediction.MeanPrediction) - currentIndex
		if futureCount > 40 {
			futureCount = 40
		}
		for i := currentIndex; i < currentIndex+futureCount && i < len(prediction.MeanPrediction); i++ {
			predictedTime := predictionTime.Add(time.Duration(i+1) * 3 * time.Minute)
			futurePredictions = append(futurePredictions, map[string]interface{}{
				"index":        i,
				"time":         predictedTime.Format(time.RFC3339),
				"predicted":    prediction.MeanPrediction[i],
				"lower_bound":  prediction.ConfidenceIntervalLower[i],
				"upper_bound":  prediction.ConfidenceIntervalUpper[i],
			})
		}
		fitData["future_predictions"] = futurePredictions
	}

	// 保存到本地文件
	fitDir := "prediction_fits"
	if err := os.MkdirAll(fitDir, 0755); err != nil {
		log.Printf("⚠️  创建拟合度目录失败: %v", err)
		return
	}

	filename := fmt.Sprintf("fit_%s_%s.json", symbol, currentTime.Format("20060102_150405"))
	filepath := filepath.Join(fitDir, filename)

	data, err := json.MarshalIndent(fitData, "", "  ")
	if err != nil {
		log.Printf("⚠️  %s 序列化预测数据失败: %v", symbol, err)
		return
	}

	if err := ioutil.WriteFile(filepath, data, 0644); err != nil {
		log.Printf("⚠️  %s 保存预测数据失败: %v", symbol, err)
		return
	}

	log.Printf("✓ %s 预测数据已保存: %s", symbol, filename)
}
