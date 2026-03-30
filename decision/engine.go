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
	"sort"
	"strings"
	"sync"
	"time"
)

// kronosPredictionEnabled 暂时关闭 Kronos 预测：不调用预测、不向 prompt 写入预测数据。改为 true 可恢复。
const kronosPredictionEnabled = false

// disableStrategyRuleValidation 为 true 时跳过 validateStrategyRules（逆大周期、追高、信号冲突等）；当前实盘与 disableLiveHardValidation 一起关闭整条 validateDecisionsWithMarketPrice 链时此项仅影响其他潜在调用路径
const disableStrategyRuleValidation = true

// disableLiveHardValidation 为 true 时：实盘解析 AI 输出后不再执行 validateDecisionsWithMarketPrice，也不执行 BTC 4h 主导性过滤；策略细节仅靠 prompt + 交易所 API 自然约束
const disableLiveHardValidation = true

// enableReflectionPass 为 true 时，第一次决策后会进行第二轮反思：将第一次决策+思维链作为输入，检查是否正确、有漏洞、是否鲁莽，输出最终决策
const enableReflectionPass = true

// reflectionSkipWhenFirstPassOnlyHoldOrWait 为 true 时：首轮 JSON 仅为 hold/wait（无 open/close）则跳过反思轮，避免「首轮已选择持有」却被二轮擅自改成平仓。
const reflectionSkipWhenFirstPassOnlyHoldOrWait = true

// reflectionMinHoldMinutesProtectProfit 盈利仓最短保护时长（分钟）：未满此时长且保证金盈亏>0，且首轮未发出 close_*，则禁止采纳反思轮新增的 close_*（与 prompt 中最小持仓纪律对齐）。
const reflectionMinHoldMinutesProtectProfit = 15

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
	StopLoss         float64 `json:"stop_loss,omitempty"`   // 开仓时设的止损价
	TakeProfit       float64 `json:"take_profit,omitempty"` // 开仓时设的止盈价
	UpdateTime       int64   `json:"update_time"`           // 持仓更新时间戳（毫秒）
}

// PositionOpenSummary 当前持仓的开仓决策概览（便于每轮决策时参考是否平仓/调仓）
type PositionOpenSummary struct {
	OpenReason string  `json:"open_reason"` // 当时因何开仓（AI 理由）
	StopLoss   float64 `json:"stop_loss"`   // 开仓时设的止损价
	TakeProfit float64 `json:"take_profit"` // 开仓时设的止盈价
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
	Symbol     string  `json:"symbol"`
	Side       string  `json:"side"` // "long" or "short"
	EntryPrice float64 `json:"entry_price"`
	ClosePrice float64 `json:"close_price"`
	Quantity   float64 `json:"quantity"`
	Leverage   int     `json:"leverage"`
	Reason     string  `json:"reason"` // "止损触发" or "止盈触发" or "未知"
}

// Context 交易上下文（传递给AI的完整信息）
type Context struct {
	CurrentTime         string                  `json:"current_time"`
	RuntimeMinutes      int                     `json:"runtime_minutes"`
	CallCount           int                     `json:"call_count"`
	TodayTradeCount     int                     `json:"today_trade_count"`      // 今天的交易次数（已废弃，使用HourlyTradeCount）
	HourlyTradeCount    int                     `json:"hourly_trade_count"`     // 最近1小时的交易次数
	HoursSinceLastTrade float64                 `json:"hours_since_last_trade"` // 距离最近一次开单的小时数（如果没有开单记录则为-1）
	Account             AccountInfo             `json:"account"`
	Positions           []PositionInfo          `json:"positions"`
	CandidateCoins      []CandidateCoin         `json:"candidate_coins"`
	MarketDataMap       map[string]*market.Data `json:"-"` // 不序列化，但内部使用
	// MarketDataOverride 非空时，fetchMarketDataForContext 直接使用此数据，跳过 API 调用（用于 prompt_simulator 回测）
	MarketDataOverride map[string]*market.Data `json:"-"`
	// SimulatorMode 为 true 时：跳过反思轮、跳过强硬验证（止损距离等），仅做基础解析，与实盘完全分离
	SimulatorMode bool `json:"-"`
	// TrainingOracleHint 仅模拟器训练：每步「复盘」锚点（因果、无未来），助 prompt 对照学习；实盘不设置
	TrainingOracleHint string `json:"-"`
	// TrainingBaselineHint 仅模拟器训练：历史自动晋升基座的综合分等指标（辅助线，非下单指令）；实盘不设置
	TrainingBaselineHint string `json:"-"`
	// TrainingWindowBenchmarkHint 仅模拟器训练：当前训练窗口内 K 线因果「方向参照」（非全局最优路径）；实盘不设置
	TrainingWindowBenchmarkHint string `json:"-"`
	// TrainingHindsightOptimalHint 仅模拟器训练：对已知回放段 15m 收盘价路径的「单笔多/空理论最大收益上界」（全知、非实盘可执行）；实盘不设置
	TrainingHindsightOptimalHint string `json:"-"`
	// TrainingDeadlockNudge 仅模拟器训练：连续多步空 JSON 数组时由 batch_trainer 注入，提醒勿再单独输出 []
	TrainingDeadlockNudge string `json:"-"`
	OITopDataMap                 map[string]*OITopData          `json:"-"` // OI Top数据映射
	PredictionMap                map[string]*PredictionData     `json:"-"` // krnos模型预测数据映射
	Performance                  interface{}                    `json:"-"` // 历史表现分析（已不再写入 prompt，仅内部保留）
	PositionOpenSummaries        map[string]PositionOpenSummary `json:"-"` // 当前持仓的开仓决策概览 key=symbol_side，便于 prompt 中输出「因何开仓、预计何时平仓」
	BTCETHLeverage               int                            `json:"-"` // BTC/ETH杠杆倍数（从配置读取）
	AltcoinLeverage              int                            `json:"-"` // 山寨币杠杆倍数（从配置读取）
	AutoClosedPositions          []AutoClosedPosition           `json:"-"` // 自动检测到的平仓信息（止损/止盈触发）
	PrecomputedIndicators        map[string]string              `json:"-"` // 决策前按配置预计算的指标 id -> 展示字符串，会写入大模型输入
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
	Warning         string  `json:"warning,omitempty"` // 警告信息（如盈亏比偏低等，不影响执行）
}

// FullDecision AI的完整决策（包含思维链）
type FullDecision struct {
	SystemPrompt     string     `json:"system_prompt"` // 系统提示词（发送给AI的系统prompt）
	UserPrompt       string     `json:"user_prompt"`   // 发送给AI的输入prompt
	CoTTrace         string     `json:"cot_trace"`     // 思维链分析（AI输出）
	Decisions        []Decision `json:"decisions"`     // 具体决策列表
	Timestamp        time.Time  `json:"timestamp"`
	FinishReason     string     `json:"finish_reason"`       // API返回的完成原因：stop（正常结束）、length（达到token限制）
	HadJSONCodeBlock bool       `json:"had_json_code_block"` // 原始响应是否包含 ```json 代码块
	UsedFallback     bool       `json:"used_fallback"`       // 是否触发过「仅输出 JSON」补调
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
	// 1.1 按配置预计算指标并写入 ctx，供 buildUserPrompt 写入大模型输入
	ComputePreDecisionIndicators(ctx)

	// 2. 构建 System Prompt（固定规则）和 User Prompt（动态数据）
	systemPrompt := buildSystemPromptWithCustom(ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, customPrompt, overrideBase, templateName)
	userPrompt := buildUserPrompt(ctx)

	// 3. 调用AI API并解析响应（带重试机制）
	maxRetries := 2 // 最多重试2次（总共3次尝试）
	var lastDecision *FullDecision
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("🔄 AI决策验证失败，正在重试 (%d/%d)，上次失败: %v", attempt, maxRetries, lastErr)
			// 重试前等待一小段时间，避免API限流
			time.Sleep(2 * time.Second)
		}

		// 调用AI API（使用 system + user prompt）
		aiResponse, finishReason, usage, err := mcpClient.CallWithMessages(systemPrompt, userPrompt)
		if err != nil {
			log.Printf("❌ 调用AI API失败: %v", err)
			if attempt == maxRetries {
				return nil, fmt.Errorf("调用AI API失败: %w", err)
			}
			lastErr = err
			continue // 网络错误，重试
		}

		// 解析AI响应（SimulatorMode 时跳过强硬验证）
		decision, err := parseFullDecisionResponse(aiResponse, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, ctx.MarketDataMap, ctx.SimulatorMode)
		hadJSONCodeBlock := hasJSONCodeBlock(aiResponse)
		if decision != nil {
			decision.HadJSONCodeBlock = hadJSONCodeBlock
		}
		needFallback := shouldFallbackForSimulator(aiResponse, decision, err, ctx.SimulatorMode)
		if needFallback {
			// 无法提取JSON或决策为空：用带上下文的简化 prompt 补调
			log.Printf("🔄 未提取到JSON决策，使用简化 prompt 补调...")
			time.Sleep(1 * time.Second)
			symbolsHint := ""
			for _, c := range ctx.CandidateCoins {
				if symbolsHint != "" {
					symbolsHint += ","
				}
				symbolsHint += c.Symbol
			}
			if symbolsHint == "" {
				symbolsHint = "BTCUSDT,ETHUSDT,DOGEUSDT"
			}
			fallbackUser := fmt.Sprintf("当前候选币种: %s。请仅输出决策JSON数组，必须以 ```json 开头。若无操作，为每个币种输出 wait，例如: ```json\n[{\"symbol\":\"BTCUSDT\",\"action\":\"wait\",\"reasoning\":\"观望\"},{\"symbol\":\"ETHUSDT\",\"action\":\"wait\",\"reasoning\":\"观望\"}]\n```。直接输出，不要其他解释。", symbolsHint)
			fallbackResp, _, _, fallbackErr := mcpClient.CallWithMessages(systemPrompt, fallbackUser)
			if fallbackErr == nil {
				if fd, fe := parseFullDecisionResponse(fallbackResp, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, ctx.MarketDataMap, true); fe == nil {
					decision = fd
					decision.HadJSONCodeBlock = hadJSONCodeBlock
					decision.UsedFallback = true
					err = nil // 补调成功，清除原错误
					if len(decision.Decisions) > 0 {
						log.Printf("   ✓ 简化 prompt 补调成功，提取到 %d 条决策", len(decision.Decisions))
					} else {
						log.Printf("   补调仍为空，响应预览: %s", truncateString(fallbackResp, 300))
					}
				}
			}
		}
		if err != nil {
			lastDecision = decision
			lastErr = err
			if ctx.SimulatorMode {
				// 测试模式：不重试，直接使用已解析的决策（若有）
				if decision != nil && len(decision.Decisions) > 0 {
					break
				}
			}
			// ⚠️ 重要：盈亏比验证已改为警告级别，不阻止决策执行
			// 如果decision已经部分填充（有Decisions），即使有盈亏比警告也允许继续
			if decision != nil && len(decision.Decisions) > 0 {
				// 检查是否只是盈亏比警告（不是真正的错误）
				errStr := err.Error()
				isRatioWarning := strings.Contains(errStr, "盈亏比") &&
					(strings.Contains(errStr, "偏低") || strings.Contains(errStr, "建议") ||
						strings.Contains(errStr, "警告") || !strings.Contains(errStr, "必须"))

				if isRatioWarning {
					log.Printf("⚠️  盈亏比警告（不影响执行）: %v，决策将继续执行", err)
					// 继续执行，不返回错误
					break
				}
			}

			// 检查是否是其他验证失败（可以重试的错误）
			errStr := err.Error()
			isValidationError := strings.Contains(errStr, "验证失败") ||
				strings.Contains(errStr, "止盈价格") ||
				strings.Contains(errStr, "风险回报比") ||
				(strings.Contains(errStr, "盈亏比") && strings.Contains(errStr, "必须"))

			if isValidationError && attempt < maxRetries {
				log.Printf("⚠️  决策验证失败（将重试）: %v", err)
				continue // 验证失败，重试
			}

			// 如果不是验证错误，或者已达到最大重试次数，返回错误
			if attempt == maxRetries {
				return decision, fmt.Errorf("解析AI响应失败: %w", err)
			}
			continue
		}

		// 成功解析，进行第二轮反思（SimulatorMode 跳过；实盘由 enableReflectionPass 控制）
		if !ctx.SimulatorMode && enableReflectionPass && !shouldSkipReflectionPass(decision) {
			reflectionDecision, reflectionErr := runReflectionPass(ctx, mcpClient, decision, systemPrompt, userPrompt)
			if reflectionErr == nil && reflectionDecision != nil {
				reflectionDecision = clampReflectionCloseDecisions(ctx, decision, reflectionDecision)
				log.Printf("🔄 反思轮完成，使用反思后的最终决策")
				decision = reflectionDecision
			} else if reflectionErr != nil {
				log.Printf("⚠️ 反思轮失败（使用第一次决策）: %v", reflectionErr)
			}
		} else if !ctx.SimulatorMode && enableReflectionPass && shouldSkipReflectionPass(decision) {
			log.Printf("⏭️ 跳过反思轮：首轮仅为 hold/wait，保持第一次决策")
		}

		// 返回结果
		decision.Timestamp = time.Now()
		decision.SystemPrompt = systemPrompt
		decision.UserPrompt = userPrompt
		decision.FinishReason = finishReason
		_ = usage // MCP 层已输出真实 token
		return decision, nil
	}

	// 所有重试都失败，返回最后一次的错误
	if lastDecision != nil {
		return lastDecision, fmt.Errorf("解析AI响应失败（已重试%d次）: %w", maxRetries, lastErr)
	}
	return nil, fmt.Errorf("调用AI API失败（已重试%d次）: %w", maxRetries, lastErr)
}

func hasJSONCodeBlock(aiResponse string) bool {
	return strings.Contains(strings.ToLower(aiResponse), "```json")
}

func shouldFallbackForSimulator(aiResponse string, decision *FullDecision, err error, simulatorMode bool) bool {
	if !simulatorMode {
		return false
	}
	if err != nil && strings.Contains(err.Error(), "提取") {
		return true
	}
	return !hasJSONCodeBlock(aiResponse) && (decision == nil || len(decision.Decisions) == 0)
}

// fetchMarketDataForContext 为上下文中的所有币种获取市场数据和OI数据
func fetchMarketDataForContext(ctx *Context) error {
	// prompt_simulator 回测：直接使用预加载的历史数据，跳过 API 调用
	if len(ctx.MarketDataOverride) > 0 {
		ctx.MarketDataMap = ctx.MarketDataOverride
		ctx.OITopDataMap = make(map[string]*OITopData)
		ctx.PredictionMap = make(map[string]*PredictionData)
		return nil
	}

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

	// 并发获取市场数据（并行请求多个币种，提升主线速度）
	positionSymbols := make(map[string]bool)
	for _, pos := range ctx.Positions {
		positionSymbols[pos.Symbol] = true
	}
	symbolsList := make([]string, 0, len(symbolSet))
	for s := range symbolSet {
		symbolsList = append(symbolsList, s)
	}
	var mu sync.Mutex
	const maxConcurrent = 8
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for _, symbol := range symbolsList {
		symbol := symbol
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			data, err := market.Get(symbol)
			if err != nil {
				return
			}
			mu.Lock()
			isExistingPosition := positionSymbols[symbol]
			if !isExistingPosition && data.OpenInterest != nil && data.CurrentPrice > 0 {
				oiValue := data.OpenInterest.Latest * data.CurrentPrice
				oiValueInMillions := oiValue / 1_000_000
				if oiValueInMillions < 15 {
					mu.Unlock()
					log.Printf("⚠️  %s 持仓价值过低(%.2fM USD < 15M)，跳过 [持仓量:%.0f × 价格:%.4f]",
						symbol, oiValueInMillions, data.OpenInterest.Latest, data.CurrentPrice)
					return
				}
			}
			ctx.MarketDataMap[symbol] = data
			mu.Unlock()
		}()
	}
	wg.Wait()

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

	// 获取krnos模型预测数据（暂时关闭：kronosPredictionEnabled = false）
	if kronosPredictionEnabled {
		for symbol, data := range ctx.MarketDataMap {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("⚠️  %s 获取预测数据时发生panic（已恢复，不影响主系统）: %v", symbol, r)
					}
				}()
				currentTrend := "sideways"
				if data.PriceChange1h > 0.5 {
					currentTrend = "up"
				} else if data.PriceChange1h < -0.5 {
					currentTrend = "down"
				}
				var priceHistory, volumeHistory []float64
				historyClient := market.NewHistoryClient()
				klines, err := historyClient.GetLatestKlinesForPrediction(symbol, "3m", 450)
				if err == nil && len(klines) >= 100 {
					priceHistory = make([]float64, len(klines))
					volumeHistory = make([]float64, len(klines))
					for i, kline := range klines {
						priceHistory[i] = kline.Close
						volumeHistory[i] = kline.Volume
					}
				}
				prediction, err := predictor.GetPredictionForSymbol(symbol, data.CurrentPrice, currentTrend, priceHistory, volumeHistory)
				if err == nil && prediction != nil {
					ctx.PredictionMap[symbol] = prediction
				}
			}()
		}
	}

	// AlphaGPT 因子已移除，决策仅依据 EMA/MACD/RSI/量能等预计算指标
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

	// 1. 加载提示词模板（核心交易策略部分）或技能库
	startLen := sb.Len()
	if templateName == "" {
		templateName = "default" // 默认使用 default 模板
	}

	// 统一使用完整模板（已移除 SkillBank 技能注入方式）
	template, err := GetPromptTemplate(templateName)
	if err != nil {
		log.Printf("⚠️  提示词模板 '%s' 不存在，使用 default: %v", templateName, err)
		template, err = GetPromptTemplate("default")
		if err != nil {
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
	sb.WriteString("1. 风险回报比: 必须 ≥ 1:1.3（冒1%风险，赚1.3%+收益）\n")
	sb.WriteString(fmt.Sprintf("2. **杠杆倍数配置（重要，必须严格遵守）**：\n"))
	sb.WriteString(fmt.Sprintf("   - **BTC/ETH杠杆倍数**: %dx（必须使用此配置的杠杆，不能使用其他值）\n", btcEthLeverage))
	sb.WriteString(fmt.Sprintf("   - **山寨币杠杆倍数**: %dx（必须使用此配置的杠杆，不能使用其他值）\n", altcoinLeverage))
	sb.WriteString("   - **⚠️ 关键**：开仓时必须根据币种使用对应的配置杠杆倍数，不能随意更改\n")
	sb.WriteString(fmt.Sprintf("   - **BTCUSDT和ETHUSDT必须使用%d倍杠杆**，其他币种必须使用%d倍杠杆\n", btcEthLeverage, altcoinLeverage))
	sb.WriteString("   - **不允许使用其他杠杆倍数**，必须严格按照配置使用\n\n")
	// 实盘固定「三等分」：每笔新开仓占用保证金 = 总权益的 1/3；名义价值 = 保证金 × 杠杆
	marginPerTrade := accountEquity / 3.0
	exampleNotionalBTC := marginPerTrade * float64(btcEthLeverage)
	sb.WriteString("3. 仓位控制（**实盘三等分铁律**，每笔新开仓统一规则）:\n")
	sb.WriteString(fmt.Sprintf("   - 当前账户总余额（总权益）: %.2f USDT\n", accountEquity))
	sb.WriteString("   - **每笔新开仓**：使用 **总余额的 1/3** 作为该笔的**保证金**（不因信号强弱改变比例；不加码、不减码）。\n")
	sb.WriteString(fmt.Sprintf("   - **本账户下每笔开仓参考**：单笔保证金 ≈ %.2f USDT（= 总余额÷3）；**position_size_usd** = 单笔保证金 × 杠杆 = 名义价值。例如 BTC/ETH 当前配置杠杆 %dx 时，open 时 position_size_usd ≈ **%.0f** USDT。\n", marginPerTrade, btcEthLeverage, exampleNotionalBTC))
	sb.WriteString("   - **同时持仓**：理论最多约 3 笔「满额」单各占 1/3；若已有持仓占用保证金，新开仓前用 **当前可用余额** 判断：若**无法再占满**「总余额÷3」这一档保证金，但**剩余可用仍够**支付「所需保证金 = position_size_usd/杠杆×1.1」且名义价值仍 ≥ 20 USDT，则 **position_size_usd 取当前可用允许的最大值（尾仓用尽剩余）**，**不要**因「差一点点不满 1/3」而一律 wait。\n")
	sb.WriteString("   - **⚠️ 关键：开仓前必须检查可用余额**\n")
	sb.WriteString("   - **可用余额检查（硬约束）**：在计算 position_size_usd 之前，必须检查可用余额是否足够\n")
	sb.WriteString("   - **计算所需保证金**：所需保证金 = position_size_usd / 杠杆倍数 × 1.1（含10%缓冲）\n")
	sb.WriteString("   - **如果可用余额 < 所需保证金**：必须 wait，禁止开仓（否则会报 -2019 Margin is insufficient）\n")
	sb.WriteString("   - **若可用余额 < 3 USDT 或 保证金率 > 90%**：禁止开新仓，必须 wait，等平仓释放保证金后再开\n")
	sb.WriteString("   - **最小订单金额（硬约束）**：订单名义价值（position_size_usd）必须 ≥ **20 USDT**（币安最小限制）\n")
	sb.WriteString("   - 若按「总余额÷3」算出的名义价值 < 20 USDT，则该账户过小，本策略无法合规下单，必须 wait 或换更大资金；**尾仓例外**：仅当剩余可用**低于**满额 1/3 保证金、且为用尽剩余时，允许该笔名义价值**低于**「总余额÷3×杠杆」，但仍须 ≥ 20 USDT，且单笔保证金**不得超过**当时可用能承受的范围内。\n")
	sb.WriteString("   - **止损距离（请在决策中遵守；系统不做代码拦截）**：以**当前价 P** 为参考：\n")
	sb.WriteString("     - 杠杆 9x 及以下：做多 stop_loss ≤ P×0.985，做空 stop_loss ≥ P×1.015；止盈与止损价差 ≥ P×8%%\n")
	sb.WriteString("     - 杠杆 10–14x：做多 stop_loss ≤ P×0.988，做空 stop_loss ≥ P×1.012；价差 ≥ P×6%%\n")
	sb.WriteString("     - 杠杆 15x+：做多 stop_loss ≤ P×0.99，做空 stop_loss ≥ P×1.01；价差 ≥ P×5%%\n")
	sb.WriteString("     - 示例：BTC 当前 72000，9x 做多时 stop_loss≤70920、take_profit≥77760（价差≥8%%）；过近止损易导致噪声扫损，请自行把握\n")
	sb.WriteString("4. 保证金: 总使用率 ≤ 90%\n")
	sb.WriteString("5. **三等分优先**：即使出现极强信号，**在满额开仓时**单笔保证金仍不得超过总余额的 1/3；**尾仓**（剩余可用不足以再占满 1/3 时）可将该笔保证金用尽**剩余可用**（系统执行时会自动与交易所可用对齐）。\n\n")
	sectionLengths["硬约束(风险控制)"] = sb.Len() - startLen

	// 2.5 基本处理逻辑与常识（通用决策流程，必须遵守）
	startLen = sb.Len()
	sb.WriteString("# 基本处理逻辑与常识（通用决策流程）\n\n")
	sb.WriteString("## ⚠️ 策略定位（必须遵守）\n\n")
	sb.WriteString("**本策略为趋势策略，以趋势为主**。不涉及「多空双开赚波动」的策略——即不同时持有多币种多空、靠价差或波动赚取利润。若出现「新开单与现有持仓方向完全相反」或「A 币多头 + B 币空头」的组合，说明决策错误，必须纠正。\n\n")
	sb.WriteString("**严禁**：在已有 A 币多头时开 B 币空头，或已有 A 币空头时开 B 币多头。开新单前必须检查现有持仓方向，**若与拟开方向相反，必须先平掉反向仓位再开新仓**；否则禁止开新仓。\n\n")
	sb.WriteString("## 一、行业常识（为何避免多币种多空背离）\n\n")
	sb.WriteString("**加密市场相关性**：山寨币与 BTC 历史数据显示强正相关（平均约 0.7），多数币种同向波动；BTC 为市场风向标。因此「A 币做多 + B 币做空」在实盘中极少成立，多为信号噪音或短期分化。\n\n")
	sb.WriteString("**同时持有多空的风险**（期货/合约通用）：① **资金压力**：双向占用保证金，资金紧张；② **交易成本**：频繁多空操作累积手续费；③ **双向亏损**：判断失误时可能多空同时亏损；④ **方向混乱**：违背「根据趋势选择单一方向」的基本原则。\n\n")
	sb.WriteString("**正确原则**：保持方向一致性。根据市场趋势做出明确判断，选择单一方向交易，避免同时持有多币种多空背离。\n\n")
	sb.WriteString("## 二、基本处理顺序（每轮决策必须遵守）\n\n")
	sb.WriteString("**1. 先看持仓，再决定开仓**\n")
	sb.WriteString("- 决策前先审视「当前持仓」：有哪些币种、多空方向、盈亏状态\n")
	sb.WriteString("- 开新仓前，先判断是否需要平仓或调仓；**先处理既有持仓，再考虑新开仓**\n\n")
	sb.WriteString("**2. 多空方向一致性（转多先平空、转空先平多）**\n")
	sb.WriteString("- **转多先平空**：若新信号确认做多（open_long），先检查是否有空仓；若有，应**优先 close_short**，再考虑 open_long\n")
	sb.WriteString("- **转空先平多**：若新信号确认做空（open_short），先检查是否有多仓；若有，应**优先 close_long**，再考虑 open_short\n")
	sb.WriteString("- 若已存在「A 币多头 + B 币空头」，应优先平掉一方使方向趋同\n\n")
	sb.WriteString("**3. 开仓前检查**\n")
	sb.WriteString("- **可用余额**：所需保证金 = position_size_usd / 杠杆 × 1.1；若可用余额 < 所需保证金，必须 wait（否则 -2019 报错）\n")
	sb.WriteString("- **保证金率 > 90% 或 可用 < 3 USDT**：禁止开新仓，必须 wait\n")
	sb.WriteString("- **同币种同向持仓**：同一币种同一方向在系统中视为**一笔仓位**；再次 open_long/open_short 视为**加仓**，会与已有仓位**合并**（数量累加、均价重算），**不会**因「条数上限」被拒。若不想加仓，应 wait 或先部分平仓。\n")
	sb.WriteString("- **持仓方向**：拟开方向与现有持仓是否冲突（见上条多空一致性）\n")
	sb.WriteString("- **信号强度**：是否符合策略开仓条件（因子、技术指标等），不确定则 wait\n\n")
	sb.WriteString("**4. 决策输出顺序**\n")
	sb.WriteString("- 同一轮中，若有平仓又有开仓，**平仓动作应排在开仓之前**（先 close_xxx，再 open_xxx）\n")
	sb.WriteString("- 例如：先 `close_short` 平掉空仓，再 `open_long` 开多\n\n")
	sectionLengths["基本处理逻辑与常识"] = sb.Len() - startLen

	// 3. 输出格式 - 动态生成
	startLen = sb.Len()
	sb.WriteString("#输出格式\n\n")
	sb.WriteString("第一步: 思维链（纯文本）\n")
	sb.WriteString("简洁分析你的思考过程\n\n")
	sb.WriteString("第二步: JSON决策数组\n\n")
	sb.WriteString("```json\n[\n")
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"position_size_usd\": %.0f, \"stop_loss\": 97000, \"take_profit\": 91000, \"risk_usd\": 300, \"reasoning\": \"下跌趋势+MACD死叉\"}\n", btcEthLeverage, exampleNotionalBTC))
	sb.WriteString("]\n```\n\n")
	sb.WriteString("**如果没有决策，输出：**\n")
	sb.WriteString("```json\n[]\n```\n\n")
	sb.WriteString("**⚠️ 再次强调：必须输出完整的JSON数组，即使为空也要输出 []，不能省略！**\n\n")
	sb.WriteString("**示例（有决策）：**\n")
	sb.WriteString("```json\n[\n")
	sb.WriteString(fmt.Sprintf("  {\"symbol\": \"BTCUSDT\", \"action\": \"open_short\", \"leverage\": %d, \"position_size_usd\": %.0f, \"stop_loss\": 97000, \"take_profit\": 91000, \"risk_usd\": 300, \"reasoning\": \"下跌趋势+MACD死叉\"},\n", btcEthLeverage, exampleNotionalBTC))
	sb.WriteString("  {\"symbol\": \"ETHUSDT\", \"action\": \"close_long\", \"reasoning\": \"止盈离场\"},\n")
	sb.WriteString("  {\"symbol\": \"HYPEUSDT\", \"action\": \"hold\", \"stop_loss\": 33.20, \"take_profit\": 35.50, \"reasoning\": \"继续持仓，上移止损保护利润，调整止盈目标\"}\n")
	sb.WriteString("]\n```\n\n")
	sb.WriteString("字段说明:\n")
	sb.WriteString("- `action`: open_long | open_short | close_long | close_short | hold | wait\n")
	sb.WriteString("- **开仓时必填**: leverage, position_size_usd, stop_loss, take_profit, risk_usd, reasoning\n")
	sb.WriteString("- **决策依据**：参考各币种下方的**预计算指标**（MACD、RSI、EMA等客观数据）；若有**历史回测**（各币种训练评估），可参考该币种胜率、夏普等。仓位根据多周期指标一致性、历史表现决定，不依赖主观信心度。\n")
	sb.WriteString(fmt.Sprintf("- **leverage字段**：必须使用配置的杠杆倍数（BTC/ETH使用%d倍，山寨币使用%d倍），不能使用其他值\n", btcEthLeverage, altcoinLeverage))
	sb.WriteString("- **hold时可选**: stop_loss, take_profit（如果提供，将更新持仓的止损止盈价格）\n")
	sb.WriteString("  - 如果市场情况变化，可以在hold时调整止损止盈以优化风险控制\n")
	sb.WriteString("  - 例如：价格上涨后，可以上移止损保护利润；或根据新的技术分析调整止盈目标\n")
	sb.WriteString("  - 如果不需要调整止损止盈，可以不提供这两个字段\n\n")
	sectionLengths["输出格式"] = sb.Len() - startLen

	// 4. 预计算指标说明（与训练一致：15m/1h/4h 的 MACD、RSI14）
	startLen = sb.Len()
	sb.WriteString("# 预计算指标说明\n\n")
	sb.WriteString("每个候选/持仓币种下方会显示**预计算指标（实时计算）**，格式如 MACD_ETH_1h=-0.05 | RSI14_ETH_1h=58.3。\n\n")
	sb.WriteString("- **MACD**：>0 多头动能 <0 空头动能，柱状图翻正/翻负可作趋势拐点参考。\n")
	sb.WriteString("- **RSI14**：>70 超买 <30 超卖，50 为多空分界；顶背离/底背离可作反转信号。\n")
	sb.WriteString("- 决策时结合 15m/1h/4h 多周期指标，至少两项信号一致再开仓。\n\n")
	sectionLengths["预计算指标说明"] = sb.Len() - startLen

	return sb.String()
}

// hasSymbolInContext 判断 symbol 是否在候选币种或持仓中
func hasSymbolInContext(ctx *Context, symbol string) bool {
	for _, c := range ctx.CandidateCoins {
		if c.Symbol == symbol {
			return true
		}
	}
	for _, p := range ctx.Positions {
		if p.Symbol == symbol {
			return true
		}
	}
	return false
}

// buildUserPrompt 构建 User Prompt（动态数据）
func buildUserPrompt(ctx *Context) string {
	var sb strings.Builder
	var sectionLengths = make(map[string]int) // 记录各部分长度

	if ctx.SimulatorMode {
		sb.WriteString("【输出格式铁律】你的回复必须包含 ```json 代码块。先输出 ```json，再写 <=5 行分析。" +
			"**禁止**仅用空数组 [] 表示观望（本步会被记为「零决策」、无法产生训练样本）；观望须输出**非空**数组，为候选币种各写一条 action 为 wait 或 hold 的对象（含 symbol 与 reasoning）。" +
			"**仅当**明确判定为 D 态（数据可疑/不可用）时，才可单独输出 []。" +
			"没有 JSON = 本步作废。\n\n")
		if strings.TrimSpace(ctx.TrainingDeadlockNudge) != "" {
			sb.WriteString(strings.TrimSpace(ctx.TrainingDeadlockNudge))
			sb.WriteString("\n\n")
		}
	}

	// 系统状态
	// 优先使用HourlyTradeCount，如果没有则使用TodayTradeCount（向后兼容）
	tradeCount := ctx.HourlyTradeCount
	if tradeCount == 0 {
		tradeCount = ctx.TodayTradeCount
	}

	startLen := sb.Len()
	// 简化系统状态显示
	// ⚠️ 关键：tradeCount 是最近1小时内的交易次数，HoursSinceLastTrade 是距离最近一次开单的小时数
	// 如果 HoursSinceLastTrade >= 1.0，说明最近一次开单在1小时前，那么最近1小时内的交易次数应该是0或更少
	if ctx.HoursSinceLastTrade >= 0 {
		sb.WriteString(fmt.Sprintf("时间:%s 周期:#%d 运行:%dm 交易:%d次 距上次:%.1fh\n\n",
			ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes, ctx.HourlyTradeCount, ctx.HoursSinceLastTrade))
	} else {
		sb.WriteString(fmt.Sprintf("时间:%s 周期:#%d 运行:%dm 交易:%d次\n\n",
			ctx.CurrentTime, ctx.CallCount, ctx.RuntimeMinutes, ctx.HourlyTradeCount))
	}
	sectionLengths["系统状态"] = sb.Len() - startLen

	// 【仅训练】每步复盘块：不含未来 K 线；帮助 prompt 对照学习；实盘 ctx 不设置本字段
	if ctx.SimulatorMode && strings.TrimSpace(ctx.TrainingOracleHint) != "" {
		sb.WriteString("## 【训练专用·每步复盘】历史截面强弱（仅当前时刻及以前的已收盘 K 线，无未来数据）\n")
		sb.WriteString("**定位**：本段等价于**轻量复盘**，帮助你在训练中对照「操作是否可改进、是否漏机会」，从而让**系统提示词在迭代中更好学习**；**未使用**当前时刻之后的任何行情。\n")
		sb.WriteString("**实盘不显示**。非下单指令，**不**替代「至少两项、JSON、风控」；与 GEPA 中的无成交步反思互补。\n\n")
		sb.WriteString(strings.TrimSpace(ctx.TrainingOracleHint))
		sb.WriteString("\n\n")
	}

	// 【仅训练】历史最优参照：仓库自动晋升基座指标，作「辅助线」对照当前会话；非未来、非下单指令
	if ctx.SimulatorMode && strings.TrimSpace(ctx.TrainingBaselineHint) != "" {
		sb.WriteString(strings.TrimSpace(ctx.TrainingBaselineHint))
		sb.WriteString("\n\n")
	}

	// 【仅训练】当前窗口 K 线因果方向参照（自上次 GEPA 后起算），防过拟合说明见段首
	if ctx.SimulatorMode && strings.TrimSpace(ctx.TrainingWindowBenchmarkHint) != "" {
		sb.WriteString(strings.TrimSpace(ctx.TrainingWindowBenchmarkHint))
		sb.WriteString("\n\n")
	}

	// 【仅训练】回放全知单笔最优上界（已知路径、非实盘承诺）
	if ctx.SimulatorMode && strings.TrimSpace(ctx.TrainingHindsightOptimalHint) != "" {
		sb.WriteString(strings.TrimSpace(ctx.TrainingHindsightOptimalHint))
		sb.WriteString("\n\n")
	}

	// 判断 BTC 是否为候选或持仓币种（仅当用户选择 BTC 时才展示）
	btcInUse := hasSymbolInContext(ctx, "BTCUSDT")

	// BTC 市场（仅当 BTC 在候选/持仓中时展示）
	startLen = sb.Len()
	if btcInUse {
		if btcData, hasBTC := ctx.MarketDataMap["BTCUSDT"]; hasBTC {
			sb.WriteString(fmt.Sprintf("BTC:%.2f 1h:%+.2f%% 4h:%+.2f%% MACD:%.4f RSI:%.2f\n\n",
				btcData.CurrentPrice, btcData.PriceChange1h, btcData.PriceChange4h,
				btcData.CurrentMACD, btcData.CurrentRSI7))
		}
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
			// 计算价格变化百分比（用于参考）
			var priceChangePct float64
			if closed.Side == "long" {
				priceChangePct = ((closed.ClosePrice - closed.EntryPrice) / closed.EntryPrice) * 100
			} else {
				priceChangePct = ((closed.EntryPrice - closed.ClosePrice) / closed.EntryPrice) * 100
			}
			// 计算保证金盈亏百分比（价格百分比 × 杠杆）
			marginPnLPct := priceChangePct * float64(closed.Leverage)

			sb.WriteString(fmt.Sprintf("- **%s %s**：入场价 %.4f，平仓价 %.4f，数量 %.4f，杠杆 %dx，价格变化 %.2f%%，保证金盈亏 %.2f%%，原因：%s\n",
				closed.Symbol, strings.ToUpper(closed.Side), closed.EntryPrice, closed.ClosePrice,
				closed.Quantity, closed.Leverage, priceChangePct, marginPnLPct, closed.Reason))
		}
		sb.WriteString("\n")
	}

	// 预计算指标按币种分组（本地最新价计算，供各币种块内联展示）
	indicatorsBySymbol := GetPrecomputedIndicatorsBySymbol(ctx)

	// 持仓（完整市场数据）
	startLen = sb.Len()
	if len(ctx.Positions) > 0 {
		// 同币种同向在执行层合并；多行仅作异常/展示延迟提示
		symbolSideCount := make(map[string]int) // "BTCUSDT_long" -> 行数
		for _, pos := range ctx.Positions {
			key := pos.Symbol + "_" + pos.Side
			symbolSideCount[key]++
		}
		sb.WriteString("## 当前持仓\n")
		sb.WriteString("**说明**：同币种同向为**一条合并仓位**；再次开立同向即**加仓**（累加数量、重算均价），**无**「同向条数上限」。若不想加仓请先 wait 或部分平仓。\n")
		anyDup := false
		for _, n := range symbolSideCount {
			if n > 1 {
				anyDup = true
				break
			}
		}
		if anyDup {
			sb.WriteString("**⚠️ 数据**：下列同品种同向在下表中出现多行（一般为展示未合并或同步延迟），决策时仍按**一笔仓位**处理：")
			parts := make([]string, 0, len(symbolSideCount))
			for k, n := range symbolSideCount {
				if n <= 1 {
					continue
				}
				sym := strings.Split(k, "_")[0]
				side := "多"
				if strings.HasSuffix(k, "_short") {
					side = "空"
				}
				parts = append(parts, fmt.Sprintf("%s %s×%d行", sym, side, n))
			}
			sort.Strings(parts)
			sb.WriteString(strings.Join(parts, "；"))
			sb.WriteString("\n\n")
		} else {
			sb.WriteString("\n")
		}
		sb.WriteString("**⚠️ 重要**：以下盈亏百分比是基于保证金的盈亏百分比（已考虑杠杆），不是价格变化百分比。\n")
		sb.WriteString("例如：价格亏损0.82%，19x杠杆 = 保证金亏损15.58%。\n\n")
		for i, pos := range ctx.Positions {
			// 简化持仓信息显示
			// ⚠️ 关键：UnrealizedPnLPct 已经是保证金百分比（考虑杠杆），不是价格百分比
			sb.WriteString(fmt.Sprintf("%d. %s %s 入场:%.4f 当前:%.4f 保证金盈亏:%.2f%%(杠杆%dx) 强平:%.4f",
				i+1, pos.Symbol, strings.ToUpper(pos.Side),
				pos.EntryPrice, pos.MarkPrice, pos.UnrealizedPnLPct,
				pos.Leverage, pos.LiquidationPrice))
			if pos.StopLoss > 0 || pos.TakeProfit > 0 {
				sb.WriteString(fmt.Sprintf(" 止损:%.4f 止盈:%.4f", pos.StopLoss, pos.TakeProfit))
			}
			sb.WriteString("\n")

			// 使用FormatMarketData输出完整市场数据
			if marketData, ok := ctx.MarketDataMap[pos.Symbol]; ok {
				sb.WriteString(market.Format(marketData))
				sb.WriteString("\n")
			}
			// 该币种预计算指标（本地最新价计算，严格对应本币种）
			if inds := indicatorsBySymbol[pos.Symbol]; len(inds) > 0 {
				sb.WriteString("**预计算指标（本地最新价计算）**: ")
				for j, ind := range inds {
					if j > 0 {
						sb.WriteString(" | ")
					}
					sb.WriteString(fmt.Sprintf("%s=%s", ind.ID, ind.Value))
				}
				sb.WriteString("\n")
			}
			// 显示krnos模型预测数据（kronosPredictionEnabled=false 时不输出）
			if kronosPredictionEnabled {
				if prediction, ok := ctx.PredictionMap[pos.Symbol]; ok {
					if marketData, ok2 := ctx.MarketDataMap[pos.Symbol]; ok2 {
						savePredictionDataSimple(pos.Symbol, marketData.CurrentPrice, prediction)
						sb.WriteString("**krnos模型预测**:\n")
						sb.WriteString(fmt.Sprintf("- **预测趋势**: %s\n", prediction.Trend))
						if len(prediction.PriceRange) >= 2 {
							sb.WriteString(fmt.Sprintf("- **价格范围**: [%.2f, %.2f]\n", prediction.PriceRange[0], prediction.PriceRange[1]))
						}
						if len(prediction.MeanPrediction) > 0 {
							outputPredictionDataPoints(&sb, prediction, marketData.CurrentPrice)
						}
						sb.WriteString("\n")
					}
				}
			}
		}
		// 当前持仓决策概览：说明每笔持仓因何开仓、预计何时平仓，便于本周期调整平仓/调仓
		if ctx.PositionOpenSummaries != nil && len(ctx.PositionOpenSummaries) > 0 {
			sb.WriteString("### 持仓决策概览（便于本周期调整平仓）\n\n")
			for i, pos := range ctx.Positions {
				posKey := pos.Symbol + "_" + pos.Side
				sum, ok := ctx.PositionOpenSummaries[posKey]
				if !ok {
					sb.WriteString(fmt.Sprintf("%d. **%s %s**：无开仓记录（可能为手动开仓），请根据当前行情决定是否平仓或调仓。\n", i+1, pos.Symbol, strings.ToUpper(pos.Side)))
					continue
				}
				sb.WriteString(fmt.Sprintf("%d. **%s %s**\n", i+1, pos.Symbol, strings.ToUpper(pos.Side)))
				sb.WriteString(fmt.Sprintf("   - **开仓原因**：%s\n", sum.OpenReason))
				if sum.StopLoss > 0 || sum.TakeProfit > 0 {
					sb.WriteString(fmt.Sprintf("   - **预计平仓**：触及止损 %.4f 或止盈 %.4f 时平仓；也可在本周期主动平仓/移动止损止盈。\n", sum.StopLoss, sum.TakeProfit))
				} else {
					sb.WriteString("   - **预计平仓**：未记录止损止盈，请根据当前行情决定平仓或设置止损止盈。\n")
				}
				sb.WriteString("\n")
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
				// 该币种预计算指标（本地最新价计算，严格对应本币种）
				if inds := indicatorsBySymbol[symbol]; len(inds) > 0 {
					sb.WriteString("**预计算指标（本地最新价计算）**: ")
					for j, ind := range inds {
						if j > 0 {
							sb.WriteString(" | ")
						}
						sb.WriteString(fmt.Sprintf("%s=%s", ind.ID, ind.Value))
					}
					sb.WriteString("\n")
				}
				// 显示krnos模型预测数据（kronosPredictionEnabled=false 时不输出）
				if kronosPredictionEnabled {
					if prediction, ok := ctx.PredictionMap[symbol]; ok {
						if marketData, ok2 := ctx.MarketDataMap[symbol]; ok2 {
							savePredictionDataSimple(symbol, marketData.CurrentPrice, prediction)
							sb.WriteString("**krnos预测**: ")
							sb.WriteString(fmt.Sprintf("趋势=%s", prediction.Trend))
							if len(prediction.PriceRange) >= 2 {
								sb.WriteString(fmt.Sprintf(" 范围=[%.2f,%.2f]", prediction.PriceRange[0], prediction.PriceRange[1]))
							}
							sb.WriteString("\n")
							if len(prediction.MeanPrediction) > 0 {
								outputPredictionDataPoints(&sb, prediction, marketData.CurrentPrice)
							}
						}
						sb.WriteString("\n")
					}
				}

				sb.WriteString("\n")
				candidateIndex++
				candidateCount++
			}
		}
		sb.WriteString("\n")
		sectionLengths[fmt.Sprintf("候选币种(%d个)", candidateCount)] = sb.Len() - startLen
	}

	// 已移除：历史表现、最近交易、历史经验总结；改为在「当前持仓」下用「持仓决策概览」说明每笔持仓因何开仓、预计何时平仓，便于大模型每轮更好调整平仓/调仓

	startLen = sb.Len()
	sb.WriteString("---\n\n")
	sb.WriteString("现在请分析并输出决策（思维链 + JSON）\n\n")
	sb.WriteString("**⚠️ 必须输出**：**禁止**只输出思维链不输出 JSON。在分析结束后，务必以 ```json 开头输出决策数组。无操作也必须输出 []。例如：\n")
	sb.WriteString("```json\n[]\n```\n（无操作时）或\n```json\n[{\"symbol\":\"BTCUSDT\",\"action\":\"hold\"}]\n```\n（有操作时）\n")
	if ctx.SimulatorMode {
		sb.WriteString("\n**⚠️ 训练模式铁律（优先于长分析）**：\n")
		sb.WriteString("① 思维链**不超过 5 行**；② **每步结尾必须**出现 ```json … ``` 代码块（含 `[]`），**禁止**只输出思维链；③ 无合法 JSON = 本步 **0 决策**，浪费训练步数、拉低可优化样本量；④ 先保证 JSON 可解析，再写分析。\n")
		sb.WriteString("⑤ **仓位表述**：同币种同向已合并为一条；**禁止**在思维链写「两个多单」「同向两笔已满」「达 2 上限」——除非上文明确列出**两行**同品种同方向持仓。\n")
	}
	sectionLengths["结尾提示"] = sb.Len() - startLen

	return sb.String()
}

// parseFullDecisionResponse 解析AI的完整决策响应
// skipStrictValidation: SimulatorMode 时为 true；实盘在 disableLiveHardValidation 时同样跳过市场侧强硬验证
func parseFullDecisionResponse(aiResponse string, accountEquity float64, btcEthLeverage, altcoinLeverage int, marketDataMap map[string]*market.Data, skipStrictValidation bool) (*FullDecision, error) {
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

	// 3. 验证决策（模拟器或 disableLiveHardValidation 时跳过：止损距离、杠杆、盈亏比、策略规则等改由 prompt 约束）
	skipMarketChecks := skipStrictValidation || disableLiveHardValidation
	if !skipMarketChecks {
		if err := validateDecisionsWithMarketPrice(decisions, accountEquity, btcEthLeverage, altcoinLeverage, marketDataMap); err != nil {
			return &FullDecision{
				CoTTrace:  cotTrace,
				Decisions: decisions,
			}, fmt.Errorf("决策验证失败: %w", err)
		}
	}

	// 4. BTC 主导性过滤（disableLiveHardValidation 时关闭，改由 prompt）
	if !disableLiveHardValidation {
		decisions = filterBTCDirectionViolations(decisions, marketDataMap)
	}

	return &FullDecision{
		CoTTrace:  cotTrace,
		Decisions: decisions,
	}, nil
}

// filterBTCDirectionViolations 基于 BTC 4h 趋势过滤违规决策（disableLiveHardValidation 时不调用）
// BTC 4h 下跌 → 非 BTC 禁止做多；BTC 4h 上涨 → 非 BTC 禁止做空
func filterBTCDirectionViolations(decisions []Decision, marketDataMap map[string]*market.Data) []Decision {
	if marketDataMap == nil {
		return decisions
	}
	btcData, ok := marketDataMap["BTCUSDT"]
	if !ok || btcData == nil || btcData.LongerTermContext == nil {
		return decisions
	}
	lt := btcData.LongerTermContext
	macd := 0.0
	if len(lt.MACDValues) > 0 {
		macd = lt.MACDValues[len(lt.MACDValues)-1]
	}
	btc4hUp := lt.EMA20 > lt.EMA50 && macd > 0
	btc4hDown := lt.EMA20 < lt.EMA50 && macd < 0

	filtered := make([]Decision, 0, len(decisions))
	for _, d := range decisions {
		if d.Symbol == "BTCUSDT" {
			filtered = append(filtered, d)
			continue
		}
		if d.Action == "open_long" && btc4hDown {
			log.Printf("🚫 [BTC主导性] %s open_long 被拒绝：BTC 4h 下跌趋势下非 BTC 禁止做多", d.Symbol)
			d.Action = "wait"
			d.Reasoning = "违反BTC主导性：BTC 4h下跌时禁止非BTC做多，改为观望"
			filtered = append(filtered, d)
			continue
		}
		if d.Action == "open_short" && btc4hUp {
			log.Printf("🚫 [BTC主导性] %s open_short 被拒绝：BTC 4h 上涨趋势下非 BTC 禁止做空", d.Symbol)
			d.Action = "wait"
			d.Reasoning = "违反BTC主导性：BTC 4h上涨时禁止非BTC做空，改为观望"
			filtered = append(filtered, d)
			continue
		}
		filtered = append(filtered, d)
	}
	return filtered
}

// shouldSkipReflectionPass 首轮无操作或仅为 hold/wait 时跳过反思（避免二轮擅自平仓）
func shouldSkipReflectionPass(first *FullDecision) bool {
	if first == nil || len(first.Decisions) == 0 {
		return true
	}
	if !reflectionSkipWhenFirstPassOnlyHoldOrWait {
		return false
	}
	for _, d := range first.Decisions {
		a := strings.ToLower(strings.TrimSpace(d.Action))
		if a != "wait" && a != "hold" && a != "" {
			return false
		}
	}
	return true
}

// firstPassRequestedClose 首轮是否已对某 symbol+方向请求平仓
func firstPassRequestedClose(first *FullDecision, symbol, side string) bool {
	if first == nil {
		return false
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	side = strings.ToLower(strings.TrimSpace(side))
	for _, d := range first.Decisions {
		ds := strings.ToUpper(strings.TrimSpace(d.Symbol))
		if ds != symbol {
			continue
		}
		a := strings.ToLower(strings.TrimSpace(d.Action))
		if side == "long" && (a == "close_long" || a == "close") {
			return true
		}
		if side == "short" && (a == "close_short" || a == "close") {
			return true
		}
	}
	return false
}

// clampReflectionCloseDecisions 剔除违规的反思平仓：盈利且持仓未满保护时长、且首轮未 close
func clampReflectionCloseDecisions(ctx *Context, first, reflected *FullDecision) *FullDecision {
	if reflected == nil || ctx == nil || len(ctx.Positions) == 0 {
		return reflected
	}
	nowMs := time.Now().UnixMilli()
	minMs := int64(reflectionMinHoldMinutesProtectProfit) * 60 * 1000
	filtered := make([]Decision, 0, len(reflected.Decisions))
	for _, d := range reflected.Decisions {
		a := strings.ToLower(strings.TrimSpace(d.Action))
		if a != "close_long" && a != "close_short" {
			filtered = append(filtered, d)
			continue
		}
		side := "long"
		if a == "close_short" {
			side = "short"
		}
		var pos *PositionInfo
		for i := range ctx.Positions {
			p := &ctx.Positions[i]
			if strings.EqualFold(p.Symbol, d.Symbol) && strings.EqualFold(p.Side, side) {
				pos = p
				break
			}
		}
		if pos == nil {
			filtered = append(filtered, d)
			continue
		}
		heldMs := nowMs - pos.UpdateTime
		if heldMs < 0 {
			heldMs = 0
		}
		profitable := pos.UnrealizedPnLPct > 0 || pos.UnrealizedPnL > 0
		if profitable && heldMs < minMs && !firstPassRequestedClose(first, d.Symbol, side) {
			log.Printf("🔒 反思平仓已拦截：%s %s 盈利且持仓 %.1f 分钟 < %d 分钟保护期，且首轮未发起 close（保持首轮纪律）",
				d.Symbol, side, float64(heldMs)/60000.0, reflectionMinHoldMinutesProtectProfit)
			continue
		}
		filtered = append(filtered, d)
	}
	if len(filtered) == len(reflected.Decisions) {
		return reflected
	}
	out := *reflected
	out.Decisions = filtered
	return &out
}

// runReflectionPass 第二轮反思：将第一次决策+思维链作为输入，检查正确性/漏洞/鲁莽性，输出最终决策
func runReflectionPass(ctx *Context, mcpClient *mcp.Client, firstDecision *FullDecision, systemPrompt, userPrompt string) (*FullDecision, error) {
	reflectionUserPrompt := buildReflectionUserPrompt(ctx, firstDecision, userPrompt)
	reflectionSystemPrompt := buildReflectionSystemPrompt(systemPrompt)

	time.Sleep(1 * time.Second) // 避免 API 限流
	aiResponse, _, _, err := mcpClient.CallWithMessages(reflectionSystemPrompt, reflectionUserPrompt)
	if err != nil {
		return nil, fmt.Errorf("反思轮API调用失败: %w", err)
	}

	skipStrict := ctx.SimulatorMode || disableLiveHardValidation
	finalDecision, err := parseFullDecisionResponse(aiResponse, ctx.Account.TotalEquity, ctx.BTCETHLeverage, ctx.AltcoinLeverage, ctx.MarketDataMap, skipStrict)
	if err != nil {
		return nil, fmt.Errorf("反思轮解析失败: %w", err)
	}
	return finalDecision, nil
}

// buildReflectionSystemPrompt 构建反思轮的系统提示（在原有基础上增加反思指令）
func buildReflectionSystemPrompt(baseSystemPrompt string) string {
	var sb strings.Builder
	sb.WriteString(baseSystemPrompt)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("# 反思轮任务说明\n\n")
	sb.WriteString("你正在进行**第二轮决策反思**。系统已给出第一次决策及其思维链，请基于以下要点进行批判性检查：\n\n")
	sb.WriteString("1. **正确性**：第一次决策是否合理？是否遗漏了重要市场信息或指标？\n")
	sb.WriteString("2. **漏洞**：是否存在风险未被充分考虑？止损止盈设置是否合理？仓位是否与风险匹配？\n")
	sb.WriteString("3. **鲁莽性**：是否过于激进（如在高不确定性下开大仓）或过于保守（如错过明确机会）？是否忽视了多周期一致性？\n\n")
	sb.WriteString("## ⚠️ 反思轮裁决优先级（必须遵守，高于「多周期逆势应平仓」的惯性判断）\n\n")
	sb.WriteString(fmt.Sprintf("- **纪律对齐**：若上方主提示词约定了「最小持仓时间」「避免随意提前平仓」，反思时**不得**仅因「1h/4h 与大周期方向不一致」就改为 `close_long` / `close_short`，除非同时满足下列**至少一条**硬性平仓条件。\n"))
	sb.WriteString("- **允许改为平仓（close）的硬性条件（至少满足一条）**：\n")
	sb.WriteString("  ① **首轮已输出**对应 `close_*` 你可加强理由，但不应无故删除平仓；\n")
	sb.WriteString("  ② **价格结构失效**：如对应周期（与开仓周期一致，通常 15m）出现实体收盘跌破/涨破关键均线或前高前低，且与持仓方向冲突；\n")
	sb.WriteString("  ③ **浮亏已显著**：基于保证金的未实现亏损已达到明显风险（例如已接近或超过计划中止损逻辑的风险暴露），不得以「轻微浮亏」即可平仓；\n")
	sb.WriteString(fmt.Sprintf("  ④ **持仓已满保护期**：持仓时长 ≥ %d 分钟**后**，若 multi-timeframe 仍与方向严重冲突且无任何改善，才允许因趋势纪律改为平仓。\n", reflectionMinHoldMinutesProtectProfit))
	sb.WriteString(fmt.Sprintf("- **盈利单保护**：持仓未满 **%d 分钟**且仍为盈利（保证金盈亏 > 0）时，**禁止**将首轮中的 `hold` / `wait` / `open_*` **改成** `close_*`，除非满足明确的结构破位（上条 ②）或首轮本身已要平仓。\n", reflectionMinHoldMinutesProtectProfit))
	sb.WriteString("- **逆势反弹单**：若首轮基于 15m 等较短周期已开仓，反思**可以**提示风险并在思维链中说明，但**默认维持执行计划**（持有至止损/止盈或结构破位），勿因「大周期仍空/仍多」单独理由清仓。\n")
	sb.WriteString("- **输出要求**：请输出你的**反思思维链**（简要说明检查结论），然后输出**最终决策 JSON**。若第一次决策已符合上述优先级，**最终 JSON 应与第一次保持一致**；仅在存在明确漏洞、鲁莽或满足硬性平仓条件时修正。\n")
	return sb.String()
}

// buildReflectionUserPrompt 构建反思轮的用户提示（原始数据 + 第一次决策 + 思维链）
func buildReflectionUserPrompt(ctx *Context, firstDecision *FullDecision, originalUserPrompt string) string {
	decisionsJSON, _ := json.MarshalIndent(firstDecision.Decisions, "", "  ")
	if len(decisionsJSON) == 0 {
		decisionsJSON = []byte("[]")
	}

	var sb strings.Builder
	if ctx != nil && len(ctx.Positions) > 0 {
		sb.WriteString("## 当前持仓时长与盈亏（反思轮必须参考；UpdateTime 为开仓时间毫秒）\n\n")
		nowMs := time.Now().UnixMilli()
		for _, p := range ctx.Positions {
			heldMin := 0.0
			if p.UpdateTime > 0 && nowMs >= p.UpdateTime {
				heldMin = float64(nowMs-p.UpdateTime) / 60000.0
			}
			sb.WriteString(fmt.Sprintf("- **%s %s**：开仓价 %.4f，标记价 %.4f，持仓约 **%.1f 分钟**，保证金盈亏 **%.2f%%**（未实现 %.4f USDT），杠杆 %dx\n",
				p.Symbol, strings.ToUpper(p.Side), p.EntryPrice, p.MarkPrice, heldMin, p.UnrealizedPnLPct, p.UnrealizedPnL, p.Leverage))
		}
		sb.WriteString("\n---\n\n")
	}
	sb.WriteString("## 原始市场数据与指标（与第一次决策时相同）\n\n")
	sb.WriteString(originalUserPrompt)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("## 第一次决策的思维链（Chain of Thought）\n\n")
	sb.WriteString(firstDecision.CoTTrace)
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("## 第一次决策的 JSON 输出\n\n")
	sb.WriteString("```json\n")
	sb.Write(decisionsJSON)
	sb.WriteString("\n```\n\n---\n\n")
	sb.WriteString("请基于以上完整信息，对第一次决策进行批判性反思，并输出**最终决策**（思维链 + JSON 数组）。\n")
	return sb.String()
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

	tryParse := func(jsonContent string) ([]Decision, bool) {
		decisions, err := parseJSONContent(jsonContent)
		return decisions, err == nil
	}

	// 方法1: 查找 ```json 代码块（支持首尾两次尝试，应对模型在中间误用```json的情况）
	for _, start := range []int{
		strings.Index(response, "```json"),
		strings.LastIndex(response, "```json"),
	} {
		if start == -1 {
			continue
		}
		contentStart := start + 7
		jsonBlockEnd := strings.Index(response[contentStart:], "```")
		if jsonBlockEnd != -1 {
			jsonContent := strings.TrimSpace(response[contentStart : contentStart+jsonBlockEnd])
			if decisions, ok := tryParse(jsonContent); ok {
				return decisions, nil
			}
		}
	}

	// 方法1b: 大小写不敏感 ```JSON
	for _, needle := range []string{"```JSON", "```Json"} {
		if start := strings.Index(response, needle); start != -1 {
			contentStart := start + len(needle)
			jsonBlockEnd := strings.Index(response[contentStart:], "```")
			if jsonBlockEnd != -1 {
				jsonContent := strings.TrimSpace(response[contentStart : contentStart+jsonBlockEnd])
				if decisions, ok := tryParse(jsonContent); ok {
					return decisions, nil
				}
			}
		}
	}

	// 方法2: 查找 ``` 代码块（可能是markdown格式）
	codeBlockStart := strings.Index(response, "```")
	if codeBlockStart != -1 {
		contentStart := codeBlockStart + 3
		for contentStart < len(response) && (response[contentStart] == ' ' || response[contentStart] == '\n' ||
			(response[contentStart] >= 'a' && response[contentStart] <= 'z')) {
			if response[contentStart] == '\n' {
				contentStart++
				break
			}
			contentStart++
		}
		codeBlockEnd := strings.Index(response[contentStart:], "```")
		if codeBlockEnd != -1 {
			jsonContent := strings.TrimSpace(response[contentStart : contentStart+codeBlockEnd])
			if decisions, ok := tryParse(jsonContent); ok {
				return decisions, nil
			}
		}
	}

	// 方法3: 直接查找JSON数组（从后往前尝试，决策数组通常在响应末尾）
	for i := len(response) - 1; i >= 0; i-- {
		if response[i] == '[' {
			arrayEnd := findMatchingBracket(response, i)
			if arrayEnd != -1 {
				jsonContent := strings.TrimSpace(response[i : arrayEnd+1])
				if decisions, ok := tryParse(jsonContent); ok && isValidDecisionArray(decisions) {
					return decisions, nil
				}
			}
		}
	}

	// 方法4: 从前往后查找（兜底）
	arrayStart := strings.Index(response, "[")
	if arrayStart != -1 {
		arrayEnd := findMatchingBracket(response, arrayStart)
		if arrayEnd != -1 {
			jsonContent := strings.TrimSpace(response[arrayStart : arrayEnd+1])
			if decisions, ok := tryParse(jsonContent); ok && isValidDecisionArray(decisions) {
				return decisions, nil
			}
		}
	}

	// 方法5: 所有方法都失败，返回错误以便触发 fallback 补调（而非静默返回空数组）
	tail := response
	if len(tail) > 800 {
		tail = "...(前省略)..." + tail[len(tail)-800:]
	}
	log.Printf("⚠️  无法提取JSON决策，响应末尾: %s", tail)
	return nil, fmt.Errorf("无法从响应中提取JSON决策")
}

// isValidDecisionArray 检查是否为有效的决策数组（排除思维链中的 [1]、[0,1] 等误匹配）
func isValidDecisionArray(decisions []Decision) bool {
	validActions := map[string]bool{
		"open_long": true, "open_short": true, "close": true,
		"close_long": true, "close_short": true, "hold": true, "wait": true,
	}
	for _, d := range decisions {
		if !validActions[d.Action] {
			return false
		}
	}
	return true
}

// decisionRaw 用于解析的中间结构，支持 reason/reasoning 别名、action 大小写、size_usd 别名
// Confidence 用 flexConfidence 以兼容 0-100 整数或 0-1 小数（如 0.65 -> 65）
type decisionRaw struct {
	Symbol          string         `json:"symbol"`
	Action          string         `json:"action"`
	Leverage        int            `json:"leverage"`
	PositionSizeUSD float64        `json:"position_size_usd"`
	SizeUSD         float64        `json:"size_usd"` // 别名，解析时若 position_size_usd 为 0 则用此值
	StopLoss        float64        `json:"stop_loss"`
	TakeProfit      float64        `json:"take_profit"`
	Confidence      flexConfidence `json:"confidence"`
	RiskUSD         float64        `json:"risk_usd"`
	Reasoning       string         `json:"reasoning"`
	Reason          string         `json:"reason"` // 别名
	Warning         string         `json:"warning"`
}

// flexConfidence 兼容 0-100 整数或 0-1 小数（0.65 -> 65）
type flexConfidence int

func (c *flexConfidence) UnmarshalJSON(data []byte) error {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case float64:
		if x > 0 && x <= 1 {
			*c = flexConfidence(x * 100)
		} else {
			*c = flexConfidence(x)
		}
	case int:
		*c = flexConfidence(x)
	default:
		*c = 0
	}
	return nil
}

// parseJSONContent 解析JSON内容（带容错处理）
func parseJSONContent(jsonContent string) ([]Decision, error) {
	// 🔧 修复常见的JSON格式错误
	jsonContent = fixMissingQuotes(jsonContent)

	var rawList []decisionRaw
	if err := json.Unmarshal([]byte(jsonContent), &rawList); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}

	decisions := make([]Decision, 0, len(rawList))
	for _, r := range rawList {
		reasoning := r.Reasoning
		if reasoning == "" {
			reasoning = r.Reason
		}
		action := strings.ToLower(strings.TrimSpace(r.Action))
		if action == "open" {
			action = "open_long" // 兼容：open 规范为 open_long
		}
		posSize := r.PositionSizeUSD
		if posSize <= 0 && r.SizeUSD > 0 {
			posSize = r.SizeUSD // 兼容 size_usd 别名
		}
		decisions = append(decisions, Decision{
			Symbol:          r.Symbol,
			Action:          action,
			Leverage:        r.Leverage,
			PositionSizeUSD: posSize,
			StopLoss:        r.StopLoss,
			TakeProfit:      r.TakeProfit,
			Confidence:      int(r.Confidence),
			RiskUSD:         r.RiskUSD,
			Reasoning:       reasoning,
			Warning:         r.Warning,
		})
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

// validateDecisionsWithMarketPrice 使用实际市场价格验证所有决策（更准确的盈亏比计算）
func validateDecisionsWithMarketPrice(decisions []Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int, marketDataMap map[string]*market.Data) error {
	for i, decision := range decisions {
		// 先进行基本验证（不包含盈亏比验证）
		if err := validateDecisionBasic(&decision, accountEquity, btcEthLeverage, altcoinLeverage); err != nil {
			return fmt.Errorf("决策 #%d 验证失败: %w", i+1, err)
		}

		// 如果是开仓操作，进行策略规则验证
		if decision.Action == "open_long" || decision.Action == "open_short" {
			// ⚠️ 关键：验证策略规则（禁止逆大周期交易、信号冲突、成交量要求、入场时机等）
			if err := validateStrategyRules(&decision, marketDataMap); err != nil {
				return fmt.Errorf("决策 #%d 策略规则验证失败: %w", i+1, err)
			}
			// 获取实际市场价格
			var currentPrice float64
			if marketDataMap != nil {
				if marketData, ok := marketDataMap[decision.Symbol]; ok && marketData != nil {
					currentPrice = marketData.CurrentPrice
				}
			}

			// 如果无法获取市场价格，尝试从market包获取
			if currentPrice <= 0 {
				if marketData, err := market.Get(decision.Symbol); err == nil {
					currentPrice = marketData.CurrentPrice
				}
			}

			if currentPrice > 0 {
				// 使用实际市场价格计算盈亏比（考虑手续费和滑点）
				// 注意：盈亏比验证改为警告级别，不阻止决策执行
				if err := validateRiskRewardRatio(&decision, currentPrice); err != nil {
					// 只有计算错误才返回错误，盈亏比偏低只是警告
					return fmt.Errorf("决策 #%d 盈亏比计算失败: %w", i+1, err)
				}
			} else {
				// 如果无法获取市场价格，使用估算方式
				log.Printf("⚠️  无法获取 %s 的市场价格，使用估算方式验证盈亏比", decision.Symbol)
				if err := validateRiskRewardRatioEstimated(&decision); err != nil {
					// 估算方式也改为警告级别
					log.Printf("⚠️  决策 #%d 盈亏比估算警告: %v", i+1, err)
					// 不返回错误，允许决策继续执行
				}
			}
		}
	}
	return nil
}

// validateDecisionBasic 基本验证（不包含盈亏比验证）
func validateDecisionBasic(d *Decision, accountEquity float64, btcEthLeverage, altcoinLeverage int) error {
	// 验证action
	validActions := map[string]bool{
		"open_long":   true,
		"open_short":  true,
		"close":       true,
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
		maxLeverage := altcoinLeverage
		if d.Symbol == "BTCUSDT" || d.Symbol == "ETHUSDT" {
			maxLeverage = btcEthLeverage
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

		// ⚠️ 关键修复：验证止损距离是否过近（防止立即触发导致毫秒级交易）
		// 需要获取当前价格来计算止损距离
		// 注意：这里使用估算价格（止损和止盈的中间值），实际验证会在validateRiskRewardRatio中使用真实价格
		var estimatedEntryPrice float64
		if d.Action == "open_long" {
			estimatedEntryPrice = d.StopLoss + (d.TakeProfit-d.StopLoss)*0.2 // 估算入场价在止损和止盈之间
		} else {
			estimatedEntryPrice = d.StopLoss - (d.StopLoss-d.TakeProfit)*0.2
		}

		var stopLossDistancePct float64
		if d.Action == "open_long" {
			stopLossDistancePct = ((estimatedEntryPrice - d.StopLoss) / estimatedEntryPrice) * 100
		} else {
			stopLossDistancePct = ((d.StopLoss - estimatedEntryPrice) / estimatedEntryPrice) * 100
		}

		// 根据杠杆倍数设置最小止损距离（改进：趋势策略建议 1.5%–3%，避免过早止损）
		// 原 0.2%–0.5% 过近，易被正常波动触发导致过早止损
		minStopLossDistancePct := 1.5 // 趋势策略默认 1.5%（与 prompt 建议一致）
		if d.Leverage >= 15 {
			minStopLossDistancePct = 1.0 // 高杠杆可略近，但至少 1%
		} else if d.Leverage >= 10 {
			minStopLossDistancePct = 1.2
		}

		if stopLossDistancePct < minStopLossDistancePct {
			return fmt.Errorf("止损距离过近（估算%.2f%% < 最小%.2f%%），可能导致立即触发导致毫秒级交易。请设置止损距离至少%.2f%%（杠杆%dx）",
				stopLossDistancePct, minStopLossDistancePct, minStopLossDistancePct, d.Leverage)
		}
	}

	return nil
}

// validateStrategyRules 验证策略规则（禁止逆大周期交易、信号冲突、成交量要求、入场时机等）
func validateStrategyRules(d *Decision, marketDataMap map[string]*market.Data) error {
	if disableStrategyRuleValidation {
		return nil // 跳过所有策略规则验证，允许 AI 自由决策
	}
	if marketDataMap == nil {
		// 如果没有市场数据，跳过策略验证（但记录警告）
		log.Printf("⚠️  %s %s: 无法获取市场数据，跳过策略规则验证", d.Symbol, d.Action)
		return nil
	}

	marketData, ok := marketDataMap[d.Symbol]
	if !ok || marketData == nil {
		// 如果无法获取该币种的市场数据，跳过策略验证（但记录警告）
		log.Printf("⚠️  %s %s: 无法获取该币种的市场数据，跳过策略规则验证", d.Symbol, d.Action)
		return nil
	}

	// 1. 验证禁止逆大周期交易（硬约束 - 最高优先级）
	// 检查4小时趋势（大周期）- 使用LongerTermContext
	if marketData.LongerTermContext != nil {
		lt := marketData.LongerTermContext
		// 判断4小时趋势：EMA20 > EMA50 且 MACD > 0 为上升趋势
		// EMA20 < EMA50 且 MACD < 0 为下降趋势
		is4hUptrend := lt.EMA20 > lt.EMA50 && len(lt.MACDValues) > 0 && lt.MACDValues[len(lt.MACDValues)-1] > 0
		is4hDowntrend := lt.EMA20 < lt.EMA50 && len(lt.MACDValues) > 0 && lt.MACDValues[len(lt.MACDValues)-1] < 0

		if d.Action == "open_long" && is4hDowntrend {
			macdValue := 0.0
			if len(lt.MACDValues) > 0 {
				macdValue = lt.MACDValues[len(lt.MACDValues)-1]
			}
			return fmt.Errorf("禁止逆大周期交易：4小时趋势向下（EMA20_4h=%.4f < EMA50_4h=%.4f, MACD_4h=%.4f < 0），严禁做多。系统拒绝执行",
				lt.EMA20, lt.EMA50, macdValue)
		}
		if d.Action == "open_short" && is4hUptrend {
			macdValue := 0.0
			if len(lt.MACDValues) > 0 {
				macdValue = lt.MACDValues[len(lt.MACDValues)-1]
			}
			return fmt.Errorf("禁止逆大周期交易：4小时趋势向上（EMA20_4h=%.4f > EMA50_4h=%.4f, MACD_4h=%.4f > 0），严禁做空。系统拒绝执行",
				lt.EMA20, lt.EMA50, macdValue)
		}
	}

	// 2. ⚠️ 关键：验证3m数据是否真实可用（必须使用真实数据）
	// 如果3m数据陈旧（WebSocket未更新），禁止基于3m拐点的开仓决策，仅允许平仓/等待
	if (d.Action == "open_long" || d.Action == "open_short") && marketData.IntradaySeries != nil {
		if marketData.IntradaySeries.IsStale {
			return fmt.Errorf("3m数据不可用（检测到数据陈旧：最近10根价格变化<0.01%%，WebSocket可能未更新）。基于3m拐点的开仓决策被禁止，仅允许平仓/等待。系统拒绝执行")
		}
	}

	// 3. 成交量要求（已移除硬约束，成交量仅作为参考指标）
	// 使用1小时成交量数据（HourlyContext）
	// ⚠️ 注意：成交量不再是开仓的硬约束，只作为参考指标
	// 如果成交量萎缩，AI应该在思维链中考虑，但不应该阻止开仓
	if marketData.HourlyContext != nil {
		hc := marketData.HourlyContext
		if hc.CurrentVolume > 0 && hc.AverageVolume > 0 {
			volumeRatio := hc.CurrentVolume / hc.AverageVolume
			// 成交量仅作为参考，记录日志但不阻止开仓
			if volumeRatio < 0.8 {
				log.Printf("⚠️  %s %s: 成交量较低（当前/平均=%.2f < 0.8），但允许开仓（成交量不再是硬约束）", d.Symbol, d.Action, volumeRatio)
			}
		}
	}

	// 4. 验证入场时机（避免追高摸顶）
	// 检查3分钟价格变化（使用IntradaySeries的MidPrices）
	if marketData.IntradaySeries != nil && len(marketData.IntradaySeries.MidPrices) >= 2 {
		prices := marketData.IntradaySeries.MidPrices
		// 计算最近3分钟的价格变化（最新价格 vs 3分钟前的价格）
		// 如果只有2个数据点，计算它们之间的变化
		if len(prices) >= 2 {
			latestPrice := prices[len(prices)-1]
			prevPrice := prices[len(prices)-2]
			if prevPrice > 0 {
				priceChange3m := ((latestPrice - prevPrice) / prevPrice) * 100

				if priceChange3m > 1.0 && d.Action == "open_long" {
					// 3分钟内价格上涨超过1%，可能是追高
					return fmt.Errorf("入场时机不佳：3分钟内价格上涨%.2f%% > 1%%，可能是追高行为，禁止开多。请等待回调确认后再入场。系统拒绝执行",
						priceChange3m)
				}
				if priceChange3m < -1.0 && d.Action == "open_short" {
					// 3分钟内价格下跌超过1%，可能是追空
					return fmt.Errorf("入场时机不佳：3分钟内价格下跌%.2f%% > 1%%，可能是追空行为，禁止开空。请等待反弹确认后再入场。系统拒绝执行",
						-priceChange3m)
				}
			}
		}
	}

	// 4. 验证止损必须设置（已在validateDecisionBasic中验证，这里再次确认）
	if d.StopLoss <= 0 {
		return fmt.Errorf("止损必须设置：stop_loss=%.4f <= 0，禁止开仓。系统拒绝执行", d.StopLoss)
	}

	// 5. 验证信号冲突处理（硬约束）
	// 检查决策reasoning中是否包含冲突信号
	reasoning := strings.ToLower(d.Reasoning)
	conflictCount := 0
	conflictSignals := []string{}

	// 检查是否同时包含"上升"和"下降"、"看多"和"看空"等冲突信号
	if (strings.Contains(reasoning, "上升") || strings.Contains(reasoning, "上涨") || strings.Contains(reasoning, "uptrend")) &&
		(strings.Contains(reasoning, "下降") || strings.Contains(reasoning, "下跌") || strings.Contains(reasoning, "downtrend")) {
		conflictCount++
		conflictSignals = append(conflictSignals, "趋势方向冲突")
	}
	if (strings.Contains(reasoning, "看多") || strings.Contains(reasoning, "做多") || strings.Contains(reasoning, "long")) &&
		(strings.Contains(reasoning, "看空") || strings.Contains(reasoning, "做空") || strings.Contains(reasoning, "short")) {
		conflictCount++
		conflictSignals = append(conflictSignals, "多空方向冲突")
	}
	if (strings.Contains(reasoning, "一致") && strings.Contains(reasoning, "不一致")) ||
		(strings.Contains(reasoning, "consistent") && strings.Contains(reasoning, "inconsistent")) {
		conflictCount++
		conflictSignals = append(conflictSignals, "周期一致性冲突")
	}
	if strings.Contains(reasoning, "多周期一致") && strings.Contains(reasoning, "多周期不一致") {
		conflictCount++
		conflictSignals = append(conflictSignals, "多周期信号冲突")
	}

	// ⚠️ 硬约束：如果信号冲突≥2条，禁止开仓
	if conflictCount >= 2 {
		return fmt.Errorf("信号冲突：检测到%d条冲突信号（%s），禁止开仓。系统拒绝执行。请重新分析市场信号，确保信号一致后再开仓",
			conflictCount, strings.Join(conflictSignals, ", "))
	}

	// 如果只有1条冲突，记录警告但不阻止（可能是AI的表述问题）
	if conflictCount == 1 {
		log.Printf("⚠️  %s %s: 检测到1条信号冲突（%s），建议重新检查决策逻辑", d.Symbol, d.Action, conflictSignals[0])
	}

	return nil
}

// validateRiskRewardRatio 使用实际市场价格验证盈亏比（考虑手续费和滑点）
// 注意：盈亏比验证改为警告级别，不阻止决策执行，而是记录警告信息
func validateRiskRewardRatio(d *Decision, currentPrice float64) error {
	const feeRate = 0.0005           // 单边手续费0.05%（币安合约Taker）
	const totalFeeRate = 0.001       // 开仓+平仓总手续费0.1%
	const slippageRate = 0.0005      // 滑点0.05%
	const recommendedRatio = 1.3     // 推荐盈亏比（不再是硬性要求）
	const minExpectedProfitPct = 0.2 // 最小预期盈利0.2%（必须覆盖手续费0.1%）

	var riskAmount, rewardAmount float64

	if d.Action == "open_long" {
		// 做多：风险 = (入场价 - 止损价) + 开仓手续费 + 止损平仓手续费 + 滑点
		priceDiff := currentPrice - d.StopLoss
		riskAmount = priceDiff + currentPrice*(feeRate+slippageRate) + d.StopLoss*(feeRate+slippageRate)

		// 做多：收益 = (止盈价 - 入场价) - 开仓手续费 - 止盈平仓手续费 - 滑点
		priceDiff = d.TakeProfit - currentPrice
		rewardAmount = priceDiff - currentPrice*(feeRate+slippageRate) - d.TakeProfit*(feeRate+slippageRate)
	} else {
		// 做空：风险 = (止损价 - 入场价) + 开仓手续费 + 止损平仓手续费 + 滑点
		priceDiff := d.StopLoss - currentPrice
		riskAmount = priceDiff + currentPrice*(feeRate+slippageRate) + d.StopLoss*(feeRate+slippageRate)

		// 做空：收益 = (入场价 - 止盈价) - 开仓手续费 - 止盈平仓手续费 - 滑点
		priceDiff = currentPrice - d.TakeProfit
		rewardAmount = priceDiff - currentPrice*(feeRate+slippageRate) - d.TakeProfit*(feeRate+slippageRate)
	}

	// 计算实际盈亏比
	if riskAmount <= 0 {
		return fmt.Errorf("风险金额计算错误（≤0），无法计算盈亏比")
	}

	actualRatio := rewardAmount / riskAmount

	// ⚠️ 关键修复：验证预期盈利是否能够覆盖手续费成本
	// 计算预期盈利百分比（价格变化百分比）
	var expectedProfitPct float64
	if d.Action == "open_long" {
		expectedProfitPct = ((d.TakeProfit - currentPrice) / currentPrice) * 100
	} else {
		expectedProfitPct = ((currentPrice - d.TakeProfit) / currentPrice) * 100
	}

	// ⚠️ 硬约束：预期盈利必须根据杠杆倍数调整，确保能覆盖手续费成本
	// 手续费相对于保证金 = 杠杆倍数 × 0.1%（开仓0.05% + 平仓0.05%，币安Taker）
	// 杠杆越高，手续费占比越高，需要更大的预期盈利
	var minExpectedProfitPctByLeverage float64
	if d.Leverage < 10 {
		minExpectedProfitPctByLeverage = 0.15 // 低杠杆：0.15%
	} else if d.Leverage < 15 {
		minExpectedProfitPctByLeverage = 0.2 // 中杠杆：0.2%
	} else {
		minExpectedProfitPctByLeverage = 0.25 // 高杠杆：0.25%
	}

	if expectedProfitPct < minExpectedProfitPctByLeverage {
		return fmt.Errorf("预期盈利过小（%.2f%% < %.2f%%），无法覆盖手续费成本（杠杆%dx，手续费占比=%.2f%%）。频繁交易会导致手续费侵蚀利润，禁止交易",
			expectedProfitPct, minExpectedProfitPctByLeverage, d.Leverage, float64(d.Leverage)*0.08)
	}

	// ⚠️ 改为警告而非错误：盈亏比计算本身不精准，应该引导"高是好"，而不是硬性要求
	// 如果盈亏比低于推荐值，记录警告信息，但不阻止决策执行
	if actualRatio < recommendedRatio {
		warningMsg := fmt.Sprintf("⚠️ 盈亏比偏低(%.2f:1，已考虑手续费和滑点)，建议≥%.1f:1以获得更好的风险收益比 [风险:%.4f 收益:%.4f] [当前价:%.4f 止损:%.4f 止盈:%.4f]",
			actualRatio, recommendedRatio, riskAmount, rewardAmount, currentPrice, d.StopLoss, d.TakeProfit)
		d.Warning = warningMsg
		log.Printf("⚠️  %s %s: %s", d.Symbol, d.Action, warningMsg)
	} else {
		log.Printf("✅ %s %s: 盈亏比 %.2f:1 符合推荐值，预期盈利%.2f%%可覆盖手续费成本", d.Symbol, d.Action, actualRatio, expectedProfitPct)
	}

	return nil
}

// validateRiskRewardRatioEstimated 使用估算方式验证盈亏比（当无法获取市场价格时）
// 注意：盈亏比验证改为警告级别，不阻止决策执行
func validateRiskRewardRatioEstimated(d *Decision) error {
	// 估算入场价（在止损和止盈之间）
	var entryPrice float64
	if d.Action == "open_long" {
		entryPrice = d.StopLoss + (d.TakeProfit-d.StopLoss)*0.2
	} else {
		entryPrice = d.StopLoss - (d.StopLoss-d.TakeProfit)*0.2
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

	// ⚠️ 改为警告而非错误：盈亏比计算本身不精准，应该引导"高是好"，而不是硬性要求
	const recommendedRatio = 1.2 // 推荐盈亏比（估算方式，因为不准确所以要求更低）
	if riskRewardRatio < recommendedRatio {
		warningMsg := fmt.Sprintf("⚠️ 估算盈亏比偏低(%.2f:1)，建议≥%.1f:1以获得更好的风险收益比 [风险:%.2f%% 收益:%.2f%%] [止损:%.4f 止盈:%.4f]",
			riskRewardRatio, recommendedRatio, riskPercent, rewardPercent, d.StopLoss, d.TakeProfit)
		d.Warning = warningMsg
		log.Printf("⚠️  %s %s: %s", d.Symbol, d.Action, warningMsg)
	} else {
		log.Printf("✅ %s %s: 估算盈亏比 %.2f:1 符合推荐值", d.Symbol, d.Action, riskRewardRatio)
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
		"close":       true,
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

		// 注意：盈亏比验证已移至validateRiskRewardRatio函数，使用实际市场价格计算
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
				"index":       i,
				"time":        predictedTime.Format(time.RFC3339),
				"predicted":   prediction.MeanPrediction[i],
				"lower_bound": prediction.ConfidenceIntervalLower[i],
				"upper_bound": prediction.ConfidenceIntervalUpper[i],
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
				"index":       i,
				"time":        predictedTime.Format(time.RFC3339),
				"predicted":   prediction.MeanPrediction[i],
				"lower_bound": prediction.ConfidenceIntervalLower[i],
				"upper_bound": prediction.ConfidenceIntervalUpper[i],
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
