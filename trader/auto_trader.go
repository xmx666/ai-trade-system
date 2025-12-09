package trader

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"nofx/decision"
	"nofx/logger"
	"nofx/market"
	"nofx/mcp"
	"nofx/pool"
	"runtime"
	"sort"
	"strings"
	"time"
)

// AutoTraderConfig 自动交易配置（简化版 - AI全权决策）
type AutoTraderConfig struct {
	// Trader标识
	ID      string // Trader唯一标识（用于日志目录等）
	Name    string // Trader显示名称
	AIModel string // AI模型: "qwen" 或 "deepseek"

	// 交易平台选择
	Exchange string // "binance", "hyperliquid" 或 "aster"

	// 币安API配置
	BinanceAPIKey    string
	BinanceSecretKey string

	// Hyperliquid配置
	HyperliquidPrivateKey string
	HyperliquidWalletAddr string
	HyperliquidTestnet    bool

	// Aster配置
	AsterUser       string // Aster主钱包地址
	AsterSigner     string // Aster API钱包地址
	AsterPrivateKey string // Aster API钱包私钥

	CoinPoolAPIURL string

	// AI配置
	UseQwen     bool
	DeepSeekKey string
	QwenKey     string

	// 自定义AI API配置
	CustomAPIURL    string
	CustomAPIKey    string
	CustomModelName string

	// 扫描配置
	ScanInterval time.Duration // 扫描间隔（建议3分钟）

	// 账户配置
	InitialBalance float64 // 初始金额（用于计算盈亏，需手动设置）

	// 杠杆配置
	BTCETHLeverage  int // BTC和ETH的杠杆倍数
	AltcoinLeverage int // 山寨币的杠杆倍数

	// 风险控制（仅作为提示，AI可自主决定）
	MaxDailyLoss    float64       // 最大日亏损百分比（提示）
	MaxDrawdown     float64       // 最大回撤百分比（提示）
	StopTradingTime time.Duration // 触发风控后暂停时长

	// 仓位模式
	IsCrossMargin bool // true=全仓模式, false=逐仓模式

	// 币种配置
	DefaultCoins []string // 默认币种列表（从数据库获取）
	TradingCoins []string // 实际交易币种列表

	// 系统提示词模板
	SystemPromptTemplate string // 系统提示词模板名称（如 "default", "aggressive"）
}

// AutoTrader 自动交易器
type AutoTrader struct {
	id                    string // Trader唯一标识
	name                  string // Trader显示名称
	aiModel               string // AI模型名称
	exchange              string // 交易平台名称
	config                AutoTraderConfig
	trader                Trader // 使用Trader接口（支持多平台）
	mcpClient             *mcp.Client
	decisionLogger        *logger.DecisionLogger // 决策日志记录器
	predictionLogger      *logger.PredictionLogger // 预测验证日志记录器
	tradeLogger           *logger.TradeLogger // 交易记录日志器
	initialBalance        float64
	dailyPnL              float64
	customPrompt          string   // 自定义交易策略prompt
	overrideBasePrompt    bool     // 是否覆盖基础prompt
	systemPromptTemplate  string   // 系统提示词模板名称
	defaultCoins          []string // 默认币种列表（从数据库获取）
	tradingCoins          []string // 实际交易币种列表
	lastResetTime         time.Time
	stopUntil             time.Time
	isRunning             bool
	startTime             time.Time        // 系统启动时间
	callCount             int              // AI调用次数
	positionFirstSeenTime map[string]int64 // 持仓首次出现时间 (symbol_side -> timestamp毫秒)
	previousPositions     map[string]map[string]interface{} // 上一个周期的持仓状态 (symbol_side -> position info)
	// 移动止损相关字段
	positionInitialStopLoss map[string]float64 // 持仓初始止损价格 (symbol_side -> stopLoss)
	positionInitialTakeProfit map[string]float64 // 持仓初始止盈价格 (symbol_side -> takeProfit) - 保留最终目标
	positionHighestProfit   map[string]float64  // 持仓最高盈利百分比 (symbol_side -> profitPct)
	positionCurrentStopLoss map[string]float64  // 持仓当前止损价格 (symbol_side -> stopLoss)
	positionHighestPrice    map[string]float64  // 持仓期间最高价格 (symbol_side -> price) 用于检测新高
	positionLowestPrice     map[string]float64  // 持仓期间最低价格 (symbol_side -> price) 用于检测新低
}

// NewAutoTrader 创建自动交易器
func NewAutoTrader(config AutoTraderConfig) (*AutoTrader, error) {
	// 设置默认值
	if config.ID == "" {
		config.ID = "default_trader"
	}
	if config.Name == "" {
		config.Name = "Default Trader"
	}
	if config.AIModel == "" {
		if config.UseQwen {
			config.AIModel = "qwen"
		} else {
			config.AIModel = "deepseek"
		}
	}

	mcpClient := mcp.New()

	// 初始化AI
	if config.AIModel == "custom" {
		// 使用自定义API
		mcpClient.SetCustomAPI(config.CustomAPIURL, config.CustomAPIKey, config.CustomModelName)
		log.Printf("🤖 [%s] 使用自定义AI API: %s (模型: %s)", config.Name, config.CustomAPIURL, config.CustomModelName)
	} else if config.UseQwen || config.AIModel == "qwen" {
		// 使用Qwen (支持自定义URL和Model)
		mcpClient.SetQwenAPIKey(config.QwenKey, config.CustomAPIURL, config.CustomModelName)
		if config.CustomAPIURL != "" || config.CustomModelName != "" {
			log.Printf("🤖 [%s] 使用阿里云Qwen AI (自定义URL: %s, 模型: %s)", config.Name, config.CustomAPIURL, config.CustomModelName)
		} else {
			log.Printf("🤖 [%s] 使用阿里云Qwen AI", config.Name)
		}
	} else {
		// 默认使用DeepSeek (支持自定义URL和Model)
		mcpClient.SetDeepSeekAPIKey(config.DeepSeekKey, config.CustomAPIURL, config.CustomModelName)
		if config.CustomAPIURL != "" || config.CustomModelName != "" {
			log.Printf("🤖 [%s] 使用DeepSeek AI (自定义URL: %s, 模型: %s)", config.Name, config.CustomAPIURL, config.CustomModelName)
		} else {
			log.Printf("🤖 [%s] 使用DeepSeek AI", config.Name)
		}
	}

	// 初始化币种池API
	if config.CoinPoolAPIURL != "" {
		pool.SetCoinPoolAPI(config.CoinPoolAPIURL)
	}

	// 设置默认交易平台
	if config.Exchange == "" {
		config.Exchange = "binance"
	}

	// 根据配置创建对应的交易器
	var trader Trader
	var err error

	// 记录仓位模式（通用）
	marginModeStr := "全仓"
	if !config.IsCrossMargin {
		marginModeStr = "逐仓"
	}
	log.Printf("📊 [%s] 仓位模式: %s", config.Name, marginModeStr)

	switch config.Exchange {
	case "binance":
		log.Printf("🏦 [%s] 使用币安合约交易", config.Name)
		trader = NewFuturesTrader(config.BinanceAPIKey, config.BinanceSecretKey)
	case "hyperliquid":
		log.Printf("🏦 [%s] 使用Hyperliquid交易", config.Name)
		trader, err = NewHyperliquidTrader(config.HyperliquidPrivateKey, config.HyperliquidWalletAddr, config.HyperliquidTestnet)
		if err != nil {
			return nil, fmt.Errorf("初始化Hyperliquid交易器失败: %w", err)
		}
	case "aster":
		log.Printf("🏦 [%s] 使用Aster交易", config.Name)
		trader, err = NewAsterTrader(config.AsterUser, config.AsterSigner, config.AsterPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("初始化Aster交易器失败: %w", err)
		}
	default:
		return nil, fmt.Errorf("不支持的交易平台: %s", config.Exchange)
	}

	// 验证初始金额配置
	if config.InitialBalance <= 0 {
		return nil, fmt.Errorf("初始金额必须大于0，请在配置中设置InitialBalance")
	}

	// 初始化决策日志记录器（使用trader ID创建独立目录）
	logDir := fmt.Sprintf("decision_logs/%s", config.ID)
	decisionLogger := logger.NewDecisionLogger(logDir)

	// 初始化预测验证日志记录器
	predictionLogDir := fmt.Sprintf("prediction_logs/%s", config.ID)
	predictionLogger := logger.NewPredictionLogger(predictionLogDir)

	// 初始化交易记录日志器（使用trader ID创建独立目录）
	tradeLogDir := fmt.Sprintf("trade_logs/%s", config.ID)
	tradeLogger := logger.NewTradeLogger(tradeLogDir)

	// 设置默认系统提示词模板
	systemPromptTemplate := config.SystemPromptTemplate
	if systemPromptTemplate == "" {
		systemPromptTemplate = "default" // 默认使用 default 模板
	}

	return &AutoTrader{
		id:                    config.ID,
		name:                  config.Name,
		aiModel:               config.AIModel,
		exchange:              config.Exchange,
		config:                config,
		trader:                trader,
		mcpClient:             mcpClient,
		decisionLogger:        decisionLogger,
		predictionLogger:      predictionLogger,
		tradeLogger:           tradeLogger,
		initialBalance:        config.InitialBalance,
		systemPromptTemplate:  systemPromptTemplate,
		defaultCoins:          config.DefaultCoins,
		tradingCoins:          config.TradingCoins,
		lastResetTime:         time.Now(),
		startTime:                time.Now(),
		callCount:                0,
		isRunning:                false,
		positionFirstSeenTime:    make(map[string]int64),
		positionInitialStopLoss:  make(map[string]float64),
		positionInitialTakeProfit: make(map[string]float64),
		positionHighestProfit:    make(map[string]float64),
		positionCurrentStopLoss:  make(map[string]float64),
		positionHighestPrice:    make(map[string]float64),
		positionLowestPrice:      make(map[string]float64),
	}, nil
}

// Run 运行自动交易主循环
func (at *AutoTrader) Run() error {
	at.isRunning = true
	log.Println("🚀 AI驱动自动交易系统启动")
	log.Printf("💰 初始余额: %.2f USDT", at.initialBalance)
	log.Printf("⚙️  扫描间隔: %v", at.config.ScanInterval)
	log.Println("🤖 AI将全权决定杠杆、仓位大小、止损止盈等参数")

	// 启动时同步币安交易记录（仅币安交易所）
	if at.exchange == "binance" {
		log.Println("📥 正在同步币安交易记录...")
		if err := at.syncBinanceTrades(); err != nil {
			log.Printf("⚠️  同步币安交易记录失败: %v（将继续运行）", err)
		} else {
			log.Println("✓ 币安交易记录同步完成")
		}
	}

	ticker := time.NewTicker(at.config.ScanInterval)
	defer ticker.Stop()

	// 首次立即执行（带panic恢复）
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ [PANIC恢复] 首次执行发生panic: %v", r)
				log.Printf("   堆栈信息: %s", getStackTrace())
			}
		}()
	if err := at.runCycle(); err != nil {
		log.Printf("❌ 执行失败: %v", err)
	}
	}()

	for at.isRunning {
		select {
		case <-ticker.C:
			// 使用panic恢复机制，防止单个周期错误导致整个循环停止
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("❌ [PANIC恢复] 交易周期发生panic: %v", r)
						log.Printf("   堆栈信息: %s", getStackTrace())
						log.Printf("   交易循环将继续运行，等待下一个周期...")
						// panic后继续运行，不退出循环
					}
				}()
			if err := at.runCycle(); err != nil {
				log.Printf("❌ 执行失败: %v", err)
					// 错误后继续运行，不退出循环
			}
			}()
		}
	}

	log.Println("⏹ 交易循环已退出")
	return nil
}

// getStackTrace 获取堆栈跟踪信息
func getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// Stop 停止自动交易
func (at *AutoTrader) Stop() {
	at.isRunning = false
	log.Println("⏹ 自动交易系统停止")
}

// runCycle 运行一个交易周期（使用AI全权决策）
func (at *AutoTrader) runCycle() error {
	at.callCount++

	log.Print("\n" + strings.Repeat("=", 70))
	log.Printf("⏰ %s - AI决策周期 #%d", time.Now().Format("2006-01-02 15:04:05"), at.callCount)
	log.Print(strings.Repeat("=", 70))

	// 创建决策记录
	record := &logger.DecisionRecord{
		ExecutionLog: []string{},
		Success:      true,
	}

	// 1. 检查是否需要停止交易
	if time.Now().Before(at.stopUntil) {
		remaining := at.stopUntil.Sub(time.Now())
		log.Printf("⏸ 风险控制：暂停交易中，剩余 %.0f 分钟", remaining.Minutes())
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("风险控制暂停中，剩余 %.0f 分钟", remaining.Minutes())
		at.decisionLogger.LogDecision(record)
		return nil
	}

	// 2. 重置日盈亏（每天重置）
	if time.Since(at.lastResetTime) > 24*time.Hour {
		at.dailyPnL = 0
		at.lastResetTime = time.Now()
		log.Println("📅 日盈亏已重置")
	}

	// 3. 检测持仓变化（自动记录止损/止盈触发的平仓）
	at.detectAndRecordPositionChanges(record)

	// 3.5. 自动移动止损检查（在收集上下文之前，先检查并调整止损）
	at.checkAndAdjustTrailingStopLoss(record)

	// 4. 收集交易上下文
	ctx, err := at.buildTradingContext()
	
	// 4.5. 将自动检测到的平仓信息传递给Context（让AI知道哪些持仓已被自动平仓）
	ctx.AutoClosedPositions = at.extractAutoClosedPositions(record)
	if err != nil {
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("构建交易上下文失败: %v", err)
		at.decisionLogger.LogDecision(record)
		return fmt.Errorf("构建交易上下文失败: %w", err)
	}

	// 保存账户状态快照
	record.AccountState = logger.AccountSnapshot{
		TotalBalance:          ctx.Account.TotalEquity,
		AvailableBalance:      ctx.Account.AvailableBalance,
		TotalUnrealizedProfit: ctx.Account.TotalPnL,
		PositionCount:         ctx.Account.PositionCount,
		MarginUsedPct:         ctx.Account.MarginUsedPct,
	}

	// 保存持仓快照
	for _, pos := range ctx.Positions {
		record.Positions = append(record.Positions, logger.PositionSnapshot{
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			PositionAmt:      pos.Quantity,
			EntryPrice:       pos.EntryPrice,
			MarkPrice:        pos.MarkPrice,
			UnrealizedProfit: pos.UnrealizedPnL,
			Leverage:         float64(pos.Leverage),
			LiquidationPrice: pos.LiquidationPrice,
		})
	}

	// 保存候选币种列表
	for _, coin := range ctx.CandidateCoins {
		record.CandidateCoins = append(record.CandidateCoins, coin.Symbol)
	}

	log.Printf("📊 账户净值: %.2f USDT | 可用: %.2f USDT | 持仓: %d",
		ctx.Account.TotalEquity, ctx.Account.AvailableBalance, ctx.Account.PositionCount)

	// 记录持仓过程中的预测验证日志
	if len(ctx.Positions) > 0 && ctx.PredictionMap != nil {
		for _, pos := range ctx.Positions {
			if prediction, ok := ctx.PredictionMap[pos.Symbol]; ok {
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
				
				// 获取当前预测值
				predictedValue := 0.0
				if currentIndex < len(prediction.MeanPrediction) {
					predictedValue = prediction.MeanPrediction[currentIndex]
				}
				
				// 记录预测验证日志
				if err := at.predictionLogger.LogPredictionValidation(
					pos.Symbol,
					pos.Side,
					pos.EntryPrice,
					pos.MarkPrice,
					predictionTime,
					currentIndex,
					predictedValue,
					at.callCount,
				); err != nil {
					log.Printf("⚠️  记录预测验证日志失败: %v", err)
				}
			}
		}

        }
	// 4. 调用AI获取完整决策
	log.Printf("🤖 正在请求AI分析并决策... [模板: %s]", at.systemPromptTemplate)
	decision, err := decision.GetFullDecisionWithCustomPrompt(ctx, at.mcpClient, at.customPrompt, at.overrideBasePrompt, at.systemPromptTemplate)

	// 即使有错误，也保存思维链、决策和输入prompt（用于debug）
	if decision != nil {
		record.SystemPrompt = decision.SystemPrompt // 保存系统提示词
		record.InputPrompt = decision.UserPrompt
		record.CoTTrace = decision.CoTTrace
		record.FinishReason = decision.FinishReason // 保存finish_reason，用于判断是否因token限制被截断
		if len(decision.Decisions) > 0 {
			decisionJSON, _ := json.MarshalIndent(decision.Decisions, "", "  ")
			record.DecisionJSON = string(decisionJSON)
		}
	}

	if err != nil {
		record.Success = false
		record.ErrorMessage = fmt.Sprintf("获取AI决策失败: %v", err)

		// 打印系统提示词和AI思维链（即使有错误，也要输出以便调试）
		if decision != nil {
			if decision.SystemPrompt != "" {
				log.Print("\n" + strings.Repeat("=", 70))
				log.Printf("📋 系统提示词 [模板: %s] (错误情况)", at.systemPromptTemplate)
				log.Println(strings.Repeat("=", 70))
				log.Println(decision.SystemPrompt)
				log.Print(strings.Repeat("=", 70) + "\n")
			}

			if decision.CoTTrace != "" {
				log.Print("\n" + strings.Repeat("-", 70))
				log.Println("💭 AI思维链分析（错误情况）:")
				log.Println(strings.Repeat("-", 70))
				log.Println(decision.CoTTrace)
				log.Print(strings.Repeat("-", 70) + "\n")
			}
		}

		at.decisionLogger.LogDecision(record)
		return fmt.Errorf("获取AI决策失败: %w", err)
	}

	// // 5. 打印系统提示词
	// log.Printf("\n" + strings.Repeat("=", 70))
	// log.Printf("📋 系统提示词 [模板: %s]", at.systemPromptTemplate)
	// log.Println(strings.Repeat("=", 70))
	// log.Println(decision.SystemPrompt)
	// log.Printf(strings.Repeat("=", 70) + "\n")

	// 6. 打印AI思维链
	// log.Printf("\n" + strings.Repeat("-", 70))
	// log.Println("💭 AI思维链分析:")
	// log.Println(strings.Repeat("-", 70))
	// log.Println(decision.CoTTrace)
	// log.Printf(strings.Repeat("-", 70) + "\n")

	// 7. 打印AI决策
	// log.Printf("📋 AI决策列表 (%d 个):\n", len(decision.Decisions))
	// for i, d := range decision.Decisions {
	// 	log.Printf("  [%d] %s: %s - %s", i+1, d.Symbol, d.Action, d.Reasoning)
	// 	if d.Action == "open_long" || d.Action == "open_short" {
	// 		log.Printf("      杠杆: %dx | 仓位: %.2f USDT | 止损: %.4f | 止盈: %.4f",
	// 			d.Leverage, d.PositionSizeUSD, d.StopLoss, d.TakeProfit)
	// 	}
	// }
	log.Println()

	// 8. 对决策排序：确保先平仓后开仓（防止仓位叠加超限）
	sortedDecisions := sortDecisionsByPriority(decision.Decisions)

	log.Println("🔄 执行顺序（已优化）: 先平仓→后开仓")
	for i, d := range sortedDecisions {
		log.Printf("  [%d] %s %s", i+1, d.Symbol, d.Action)
	}
	log.Println()

	// 执行决策并记录结果
	for _, d := range sortedDecisions {
		actionRecord := logger.DecisionAction{
			Action:    d.Action,
			Symbol:    d.Symbol,
			Quantity:  0,
			Leverage:  d.Leverage,
			Price:     0,
			Timestamp: time.Now(),
			Success:   false,
		}

		if err := at.executeDecisionWithRecord(&d, &actionRecord); err != nil {
			log.Printf("❌ 执行决策失败 (%s %s): %v", d.Symbol, d.Action, err)
			actionRecord.Error = err.Error()
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("❌ %s %s 失败: %v", d.Symbol, d.Action, err))
		} else {
			actionRecord.Success = true
			record.ExecutionLog = append(record.ExecutionLog, fmt.Sprintf("✓ %s %s 成功", d.Symbol, d.Action))
			// 成功执行后短暂延迟
			time.Sleep(1 * time.Second)
		}

		record.Decisions = append(record.Decisions, actionRecord)
	}

	// 9. 更新持仓状态（用于下次周期检测持仓变化）
	at.updatePreviousPositions()

	// 10. 保存决策记录
	if err := at.decisionLogger.LogDecision(record); err != nil {
		log.Printf("⚠ 保存决策记录失败: %v", err)
	}

	return nil
}

// buildTradingContext 构建交易上下文
func (at *AutoTrader) buildTradingContext() (*decision.Context, error) {
	// 1. 获取账户信息
	balance, err := at.trader.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("获取账户余额失败: %w", err)
	}

	// 获取账户字段
	totalWalletBalance := 0.0
	totalUnrealizedProfit := 0.0
	availableBalance := 0.0

	if wallet, ok := balance["totalWalletBalance"].(float64); ok {
		totalWalletBalance = wallet
	}
	if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
		totalUnrealizedProfit = unrealized
	}
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Total Equity = 钱包余额 + 未实现盈亏
	totalEquity := totalWalletBalance + totalUnrealizedProfit

	// 2. 获取持仓信息
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	var positionInfos []decision.PositionInfo
	totalMarginUsed := 0.0

	// 当前持仓的key集合（用于清理已平仓的记录）
	currentPositionKeys := make(map[string]bool)

	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity // 空仓数量为负，转为正数
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)

		// 计算占用保证金（估算）
		leverage := 10 // 默认值，实际应该从持仓信息获取
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		marginUsed := (quantity * markPrice) / float64(leverage)
		totalMarginUsed += marginUsed

		// 计算盈亏百分比（基于保证金，考虑杠杆）
		// ⚠️ 重要：UnrealizedPnLPct 应该基于保证金百分比，而不是价格百分比
		// 例如：价格亏损2%，10x杠杆 = 保证金亏损20%
		pnlPct := 0.0
		if marginUsed > 0 {
			pnlPct = (unrealizedPnl / marginUsed) * 100
		} else {
			// 如果无法计算保证金，回退到价格百分比（但这种情况不应该发生）
			if side == "long" {
				pnlPct = ((markPrice - entryPrice) / entryPrice) * 100
			} else {
				pnlPct = ((entryPrice - markPrice) / entryPrice) * 100
			}
		}

		// 跟踪持仓首次出现时间（用于计算持仓时长）
		// 注意：由于交易所API不提供开仓时间，我们使用首次发现时间作为近似值
		// 这可能在系统重启后不准确，但对于运行中的系统是准确的
		posKey := symbol + "_" + side
		currentPositionKeys[posKey] = true
		now := time.Now().UnixMilli()
		var updateTime int64
		if firstSeenTime, exists := at.positionFirstSeenTime[posKey]; exists {
			// 持仓已存在，使用首次发现时间
			updateTime = firstSeenTime
		} else {
			// 新持仓，记录当前时间作为首次发现时间
			at.positionFirstSeenTime[posKey] = now
			updateTime = now
		}

		positionInfos = append(positionInfos, decision.PositionInfo{
			Symbol:           symbol,
			Side:             side,
			EntryPrice:       entryPrice,
			MarkPrice:        markPrice,
			Quantity:         quantity,
			Leverage:         leverage,
			UnrealizedPnL:    unrealizedPnl,
			UnrealizedPnLPct: pnlPct,
			LiquidationPrice: liquidationPrice,
			MarginUsed:       marginUsed,
			UpdateTime:       updateTime,
		})
	}

	// 清理已平仓的持仓记录
	for key := range at.positionFirstSeenTime {
		if !currentPositionKeys[key] {
			delete(at.positionFirstSeenTime, key)
		}
	}

	// 3. 获取交易员的候选币种池
	candidateCoins, err := at.getCandidateCoins()
	if err != nil {
		return nil, fmt.Errorf("获取候选币种失败: %w", err)
	}

	// 4. 计算总盈亏
	totalPnL := totalEquity - at.initialBalance
	totalPnLPct := 0.0
	if at.initialBalance > 0 {
		totalPnLPct = (totalPnL / at.initialBalance) * 100
	}

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	// 5. 分析历史表现（最近100个周期，避免长期持仓的交易记录丢失）
	// 假设每3分钟一个周期，100个周期 = 5小时，足够覆盖大部分交易
	performance, err := at.decisionLogger.AnalyzePerformance(100)
	if err != nil {
		log.Printf("⚠️  分析历史表现失败: %v", err)
		// 不影响主流程，继续执行（但设置performance为nil以避免传递错误数据）
		performance = nil
	}

	// 5.5 计算最近1小时的交易次数
	hourlyTradeCount := 0
	hourlyTradeCount, err = at.decisionLogger.GetHourlyTradeCount()
	if err != nil {
		log.Printf("⚠️  获取最近1小时交易次数失败: %v", err)
		// 不影响主流程，继续执行（设置为0）
		hourlyTradeCount = 0
	}

	// 向后兼容：也计算今天的交易次数
	todayTradeCount := 0
	todayTradeCount, _ = at.decisionLogger.GetTodayTradeCount()
	// 5.6 计算距离最近一次开单的小时数
	hoursSinceLastTrade := -1.0
	hoursSinceLastTrade, err = at.decisionLogger.GetLastOpenTradeTime()
	if err != nil {
		log.Printf("⚠️  获取最近一次开单时间失败: %v", err)
		// 不影响主流程，继续执行（设置为-1表示没有记录）
		hoursSinceLastTrade = -1
	}


	// 6. 构建上下文
	ctx := &decision.Context{
		CurrentTime:     time.Now().Format("2006-01-02 15:04:05"),
		RuntimeMinutes:  int(time.Since(at.startTime).Minutes()),
		CallCount:       at.callCount,
		TodayTradeCount: todayTradeCount, // 今天的交易次数（向后兼容）
		HourlyTradeCount: hourlyTradeCount, // 最近1小时的交易次数
		HoursSinceLastTrade: hoursSinceLastTrade, // 距离最近一次开单的小时数
		BTCETHLeverage:  at.config.BTCETHLeverage,  // 使用配置的杠杆倍数
		AltcoinLeverage: at.config.AltcoinLeverage, // 使用配置的杠杆倍数
		Account: decision.AccountInfo{
			TotalEquity:      totalEquity,
			AvailableBalance: availableBalance,
			TotalPnL:         totalPnL,
			TotalPnLPct:      totalPnLPct,
			MarginUsed:       totalMarginUsed,
			MarginUsedPct:    marginUsedPct,
			PositionCount:    len(positionInfos),
		},
		Positions:      positionInfos,
		CandidateCoins: candidateCoins,
		Performance:    performance, // 添加历史表现分析
	}

	return ctx, nil
}

// executeDecisionWithRecord 执行AI决策并记录详细信息
func (at *AutoTrader) executeDecisionWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	switch decision.Action {
	case "open_long":
		return at.executeOpenLongWithRecord(decision, actionRecord)
	case "open_short":
		return at.executeOpenShortWithRecord(decision, actionRecord)
	case "close_long":
		return at.executeCloseLongWithRecord(decision, actionRecord)
	case "close_short":
		return at.executeCloseShortWithRecord(decision, actionRecord)
	case "hold":
		// hold时可能需要调整止损止盈
		return at.executeHoldWithRecord(decision, actionRecord)
	case "wait":
		// wait无需执行，仅记录
		return nil
	default:
		return fmt.Errorf("未知的action: %s", decision.Action)
	}
}

// executeOpenLongWithRecord 执行开多仓并记录详细信息
func (at *AutoTrader) executeOpenLongWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  📈 开多仓: %s", decision.Symbol)

	// 获取当前价格（用于计算数量）
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}

	// 计算数量
	quantity := decision.PositionSizeUSD / marketData.CurrentPrice
	actionRecord.Quantity = quantity

	// 验证订单名义价值（币安要求 >= 20 USDT）
	const minOrderNotional = 20.0
	orderNotional := decision.PositionSizeUSD
	if orderNotional < minOrderNotional {
		return fmt.Errorf("订单名义价值 %.2f USDT 小于币安最小限制 %.0f USDT，无法开仓。请增加仓位大小", orderNotional, minOrderNotional)
	}

	// 检查保证金是否充足
	balance, err := at.trader.GetBalance()
	if err != nil {
		return fmt.Errorf("获取账户余额失败，无法检查保证金: %w", err)
	}

	availableBalance := 0.0
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// 计算开仓所需保证金 = 仓位价值 / 杠杆
	requiredMargin := orderNotional / float64(decision.Leverage)
	// 留10%的缓冲，确保有足够保证金
	requiredMarginWithBuffer := requiredMargin * 1.1

	if availableBalance < requiredMarginWithBuffer {
		// 如果这是一个特别好的机会（信心度 >= 90），尝试平掉表现较差的仓位
		if decision.Confidence >= 90 {
			log.Printf("  💡 保证金不足，但这是一个高信心度机会(信心度: %d%%)，尝试平掉表现较差的仓位释放保证金", decision.Confidence)
			
			closedMargin, err := at.closeWorstPositionsToFreeMargin(requiredMarginWithBuffer-availableBalance, decision.Symbol)
			if err != nil {
				log.Printf("  ⚠️ 尝试平仓释放保证金失败: %v", err)
				return fmt.Errorf("保证金不足，无法开仓。可用余额: %.2f USDT, 所需保证金(含缓冲): %.2f USDT (仓位价值: %.2f USDT, 杠杆: %dx)", 
					availableBalance, requiredMarginWithBuffer, orderNotional, decision.Leverage)
			}

			if closedMargin > 0 {
				log.Printf("  ✅ 已平掉表现较差的仓位，释放保证金: %.2f USDT", closedMargin)
				// 重新获取余额
				balance, err = at.trader.GetBalance()
				if err == nil {
					if avail, ok := balance["availableBalance"].(float64); ok {
						availableBalance = avail
					}
				}

				// 再次检查保证金
				if availableBalance < requiredMarginWithBuffer {
					return fmt.Errorf("平仓后保证金仍不足。可用余额: %.2f USDT, 所需保证金(含缓冲): %.2f USDT", 
						availableBalance, requiredMarginWithBuffer)
				}
			} else {
				return fmt.Errorf("保证金不足，且无法通过平仓释放足够保证金。可用余额: %.2f USDT, 所需保证金(含缓冲): %.2f USDT", 
					availableBalance, requiredMarginWithBuffer)
			}
		} else {
			return fmt.Errorf("保证金不足，无法开仓。可用余额: %.2f USDT, 所需保证金(含缓冲): %.2f USDT (仓位价值: %.2f USDT, 杠杆: %dx)", 
				availableBalance, requiredMarginWithBuffer, orderNotional, decision.Leverage)
		}
	}

	log.Printf("  💰 保证金检查通过: 可用余额 %.2f USDT, 所需保证金 %.2f USDT (仓位价值 %.2f USDT, 杠杆 %dx)", 
		availableBalance, requiredMargin, orderNotional, decision.Leverage)

	// 设置仓位模式
	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		log.Printf("  ⚠️ 设置仓位模式失败: %v", err)
		// 继续执行，不影响交易
	}

	// 开仓
	order, err := at.trader.OpenLong(decision.Symbol, quantity, decision.Leverage)
	if err != nil {
		return err
	}

	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	log.Printf("  ✓ 开仓成功，订单ID: %v, 数量: %.4f", order["orderId"], quantity)

	// ⚠️ 关键：开仓后立即从持仓信息获取实际入场价格（entryPrice）
	// 等待一小段时间确保订单已成交
	time.Sleep(500 * time.Millisecond)
	positions, err := at.trader.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
				if entryPrice, ok := pos["entryPrice"].(float64); ok && entryPrice > 0 {
					actionRecord.Price = entryPrice
					// 更新实际数量（可能因为精度问题略有差异）
					if posAmt, ok := pos["positionAmt"].(float64); ok && posAmt > 0 {
						actionRecord.Quantity = posAmt
					}
					log.Printf("  📊 实际入场价格: %.4f, 实际数量: %.4f", entryPrice, actionRecord.Quantity)
					break
				}
			}
		}
	}

	// 如果无法从持仓获取，使用市场价格作为备选
	if actionRecord.Price == 0 {
		actionRecord.Price = marketData.CurrentPrice
		log.Printf("  ⚠️ 无法获取实际入场价格，使用市场价格: %.4f", marketData.CurrentPrice)
	}

	// 记录开仓时间
	posKey := decision.Symbol + "_long"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// 设置止损止盈
	if err := at.trader.SetStopLoss(decision.Symbol, "LONG", quantity, decision.StopLoss); err != nil {
		log.Printf("  ⚠ 设置止损失败: %v", err)
	} else {
		// 记录初始止损价格和当前止损价格
		at.positionInitialStopLoss[posKey] = decision.StopLoss
		at.positionCurrentStopLoss[posKey] = decision.StopLoss
		at.positionHighestProfit[posKey] = 0.0 // 初始盈利为0
		// 初始化最高/最低价格记录（使用实际入场价格）
		at.positionHighestPrice[posKey] = actionRecord.Price
		at.positionLowestPrice[posKey] = actionRecord.Price
		log.Printf("  📝 记录初始止损价格: %.4f, 初始价格: %.4f", decision.StopLoss, actionRecord.Price)
	}
	if err := at.trader.SetTakeProfit(decision.Symbol, "LONG", quantity, decision.TakeProfit); err != nil {
		log.Printf("  ⚠ 设置止盈失败: %v", err)
	} else {
		// 记录初始止盈价格（最终目标，让利润继续奔跑）
		at.positionInitialTakeProfit[posKey] = decision.TakeProfit
		log.Printf("  📝 记录初始止盈价格（最终目标）: %.4f", decision.TakeProfit)
	}

	// ⚠️ 关键：记录开仓交易到本地文件
	if at.tradeLogger != nil {
		orderID := int64(0)
		if id, ok := order["orderId"].(int64); ok {
			orderID = id
		}
		_, err := at.tradeLogger.RecordOpenTrade(
			decision.Symbol,
			"long",
			actionRecord.Price,
			actionRecord.Quantity,
			decision.Leverage,
			orderID,
			decision.StopLoss,
			decision.TakeProfit,
		)
		if err != nil {
			log.Printf("  ⚠️ 记录开仓交易失败: %v", err)
		} else {
			log.Printf("  ✅ 开仓交易已记录到本地文件")
		}
	}

	return nil
}

// executeOpenShortWithRecord 执行开空仓并记录详细信息
func (at *AutoTrader) executeOpenShortWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  📉 开空仓: %s", decision.Symbol)

	// 获取当前价格（用于计算数量）
	marketData, err := market.Get(decision.Symbol)
	if err != nil {
		return err
	}

	// 计算数量
	quantity := decision.PositionSizeUSD / marketData.CurrentPrice
	actionRecord.Quantity = quantity

	// 验证订单名义价值（币安要求 >= 20 USDT）
	const minOrderNotional = 20.0
	orderNotional := decision.PositionSizeUSD
	if orderNotional < minOrderNotional {
		return fmt.Errorf("订单名义价值 %.2f USDT 小于币安最小限制 %.0f USDT，无法开仓。请增加仓位大小", orderNotional, minOrderNotional)
	}

	// 检查保证金是否充足
	balance, err := at.trader.GetBalance()
	if err != nil {
		return fmt.Errorf("获取账户余额失败，无法检查保证金: %w", err)
	}

	availableBalance := 0.0
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// 计算开仓所需保证金 = 仓位价值 / 杠杆
	requiredMargin := orderNotional / float64(decision.Leverage)
	// 留10%的缓冲，确保有足够保证金
	requiredMarginWithBuffer := requiredMargin * 1.1

	if availableBalance < requiredMarginWithBuffer {
		// 如果这是一个特别好的机会（信心度 >= 90），尝试平掉表现较差的仓位
		if decision.Confidence >= 90 {
			log.Printf("  💡 保证金不足，但这是一个高信心度机会(信心度: %d%%)，尝试平掉表现较差的仓位释放保证金", decision.Confidence)
			
			closedMargin, err := at.closeWorstPositionsToFreeMargin(requiredMarginWithBuffer-availableBalance, decision.Symbol)
			if err != nil {
				log.Printf("  ⚠️ 尝试平仓释放保证金失败: %v", err)
				return fmt.Errorf("保证金不足，无法开仓。可用余额: %.2f USDT, 所需保证金(含缓冲): %.2f USDT (仓位价值: %.2f USDT, 杠杆: %dx)", 
					availableBalance, requiredMarginWithBuffer, orderNotional, decision.Leverage)
			}

			if closedMargin > 0 {
				log.Printf("  ✅ 已平掉表现较差的仓位，释放保证金: %.2f USDT", closedMargin)
				// 重新获取余额
				balance, err = at.trader.GetBalance()
				if err == nil {
					if avail, ok := balance["availableBalance"].(float64); ok {
						availableBalance = avail
					}
				}

				// 再次检查保证金
				if availableBalance < requiredMarginWithBuffer {
					return fmt.Errorf("平仓后保证金仍不足。可用余额: %.2f USDT, 所需保证金(含缓冲): %.2f USDT", 
						availableBalance, requiredMarginWithBuffer)
				}
			} else {
				return fmt.Errorf("保证金不足，且无法通过平仓释放足够保证金。可用余额: %.2f USDT, 所需保证金(含缓冲): %.2f USDT", 
					availableBalance, requiredMarginWithBuffer)
			}
		} else {
			return fmt.Errorf("保证金不足，无法开仓。可用余额: %.2f USDT, 所需保证金(含缓冲): %.2f USDT (仓位价值: %.2f USDT, 杠杆: %dx)", 
				availableBalance, requiredMarginWithBuffer, orderNotional, decision.Leverage)
		}
	}

	log.Printf("  💰 保证金检查通过: 可用余额 %.2f USDT, 所需保证金 %.2f USDT (仓位价值 %.2f USDT, 杠杆 %dx)", 
		availableBalance, requiredMargin, orderNotional, decision.Leverage)

	// 设置仓位模式
	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		log.Printf("  ⚠️ 设置仓位模式失败: %v", err)
		// 继续执行，不影响交易
	}

	// 开仓
	order, err := at.trader.OpenShort(decision.Symbol, quantity, decision.Leverage)
	if err != nil {
		return err
	}

	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	log.Printf("  ✓ 开仓成功，订单ID: %v, 数量: %.4f", order["orderId"], quantity)

	// ⚠️ 关键：开仓后立即从持仓信息获取实际入场价格（entryPrice）
	// 等待一小段时间确保订单已成交
	time.Sleep(500 * time.Millisecond)
	positions, err := at.trader.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
				if entryPrice, ok := pos["entryPrice"].(float64); ok && entryPrice > 0 {
					actionRecord.Price = entryPrice
					// 更新实际数量（可能因为精度问题略有差异）
					if posAmt, ok := pos["positionAmt"].(float64); ok && posAmt > 0 {
						actionRecord.Quantity = posAmt
					}
					log.Printf("  📊 实际入场价格: %.4f, 实际数量: %.4f", entryPrice, actionRecord.Quantity)
					break
				}
			}
		}
	}

	// 如果无法从持仓获取，使用市场价格作为备选
	if actionRecord.Price == 0 {
		actionRecord.Price = marketData.CurrentPrice
		log.Printf("  ⚠️ 无法获取实际入场价格，使用市场价格: %.4f", marketData.CurrentPrice)
	}

	// 记录开仓时间
	posKey := decision.Symbol + "_short"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// 设置止损止盈
	if err := at.trader.SetStopLoss(decision.Symbol, "SHORT", quantity, decision.StopLoss); err != nil {
		log.Printf("  ⚠ 设置止损失败: %v", err)
	} else {
		// 记录初始止损价格和当前止损价格
		at.positionInitialStopLoss[posKey] = decision.StopLoss
		at.positionCurrentStopLoss[posKey] = decision.StopLoss
		at.positionHighestProfit[posKey] = 0.0 // 初始盈利为0
		// 初始化最高/最低价格记录（使用实际入场价格）
		at.positionHighestPrice[posKey] = actionRecord.Price
		at.positionLowestPrice[posKey] = actionRecord.Price
		log.Printf("  📝 记录初始止损价格: %.4f, 初始价格: %.4f", decision.StopLoss, actionRecord.Price)
	}
	if err := at.trader.SetTakeProfit(decision.Symbol, "SHORT", quantity, decision.TakeProfit); err != nil {
		log.Printf("  ⚠ 设置止盈失败: %v", err)
	} else {
		// 记录初始止盈价格（最终目标，让利润继续奔跑）
		at.positionInitialTakeProfit[posKey] = decision.TakeProfit
		log.Printf("  📝 记录初始止盈价格（最终目标）: %.4f", decision.TakeProfit)
	}

	// ⚠️ 关键：记录开仓交易到本地文件
	if at.tradeLogger != nil {
		orderID := int64(0)
		if id, ok := order["orderId"].(int64); ok {
			orderID = id
		}
		_, err := at.tradeLogger.RecordOpenTrade(
			decision.Symbol,
			"short",
			actionRecord.Price,
			actionRecord.Quantity,
			decision.Leverage,
			orderID,
			decision.StopLoss,
			decision.TakeProfit,
		)
		if err != nil {
			log.Printf("  ⚠️ 记录开仓交易失败: %v", err)
		} else {
			log.Printf("  ✅ 开仓交易已记录到本地文件")
		}
	}

	return nil
}

// executeCloseLongWithRecord 执行平多仓并记录详细信息
func (at *AutoTrader) executeCloseLongWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 平多仓: %s", decision.Symbol)

	// 平仓
	order, err := at.trader.CloseLong(decision.Symbol, 0) // 0 = 全部平仓
	if err != nil {
		return err
	}

	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// ⚠️ 关键：平仓后等待一小段时间，然后从持仓信息或市场价格获取实际平仓价格
	// 由于平仓后持仓已不存在，我们需要从市场价格或订单信息中获取
	time.Sleep(500 * time.Millisecond)
	
	// 尝试从订单返回结果中获取实际执行价格
	if execPrice, ok := order["executionPrice"].(float64); ok && execPrice > 0 {
		actionRecord.Price = execPrice
		log.Printf("  📊 实际平仓价格（从订单）: %.4f", execPrice)
	} else if avgPrice, ok := order["avgPrice"].(float64); ok && avgPrice > 0 {
		actionRecord.Price = avgPrice
		log.Printf("  📊 实际平仓价格（平均价格）: %.4f", avgPrice)
	} else {
		// 如果订单返回结果中没有价格，使用平仓后的市场价格作为备选
		// 注意：这可能不够准确，但比平仓前的价格更接近实际执行价格
		marketData, err := market.Get(decision.Symbol)
		if err == nil {
			actionRecord.Price = marketData.CurrentPrice
			log.Printf("  ⚠️ 无法获取实际平仓价格，使用平仓后市场价格: %.4f", marketData.CurrentPrice)
		} else {
			// 最后的备选：使用平仓前的市场价格
			marketData, _ := market.Get(decision.Symbol)
			if marketData != nil {
				actionRecord.Price = marketData.CurrentPrice
				log.Printf("  ⚠️ 使用市场价格作为备选: %.4f", marketData.CurrentPrice)
			}
		}
	}

	// 取消所有挂单
	if err := at.trader.CancelAllOrders(decision.Symbol); err != nil {
		log.Printf("  ⚠ 取消挂单失败: %v", err)
	}

	log.Printf("  ✓ 平仓成功，平仓价格: %.4f", actionRecord.Price)

	// ⚠️ 关键：更新平仓交易记录到本地文件
	if at.tradeLogger != nil {
		orderID := int64(0)
		if id, ok := order["orderId"].(int64); ok {
			orderID = id
		}
		_, err := at.tradeLogger.UpdateCloseTrade(
			decision.Symbol,
			"long",
			actionRecord.Price,
			actionRecord.Quantity,
			"ai_decision", // AI决策平仓
			orderID,
		)
		if err != nil {
			log.Printf("  ⚠️ 更新平仓交易记录失败: %v", err)
		} else {
			log.Printf("  ✅ 平仓交易已更新到本地文件")
		}
	}

	return nil
}

// executeHoldWithRecord 执行hold决策并记录详细信息（可能调整止损止盈）
func (at *AutoTrader) executeHoldWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 持仓: %s", decision.Symbol)
	
	// 检查是否有持仓
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("获取持仓失败: %w", err)
	}
	
	// 查找对应的持仓
	var position map[string]interface{}
	var side string
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol {
			posSide := pos["side"].(string)
			posKey := decision.Symbol + "_" + posSide
			position = pos
			side = posSide
			
			// 如果提供了止损或止盈，更新它们
			quantity := pos["positionAmt"].(float64)
			if quantity < 0 {
				quantity = -quantity
			}
			
			// 更新止损
			if decision.StopLoss > 0 {
				sideUpper := strings.ToUpper(side)
				if err := at.trader.SetStopLoss(decision.Symbol, sideUpper, quantity, decision.StopLoss); err != nil {
					log.Printf("  ⚠️  %s 更新止损失败: %v", decision.Symbol, err)
					// 继续执行，不返回错误（止损更新失败不影响hold操作）
				} else {
					// 更新记录的止损价格
					at.positionCurrentStopLoss[posKey] = decision.StopLoss
					log.Printf("  📝 %s %s 止损已更新: %.4f", decision.Symbol, sideUpper, decision.StopLoss)
				}
			}
			
			// 更新止盈
			if decision.TakeProfit > 0 {
				sideUpper := strings.ToUpper(side)
				if err := at.trader.SetTakeProfit(decision.Symbol, sideUpper, quantity, decision.TakeProfit); err != nil {
					log.Printf("  ⚠️  %s 更新止盈失败: %v", decision.Symbol, err)
					// 继续执行，不返回错误（止盈更新失败不影响hold操作）
				} else {
					log.Printf("  📝 %s %s 止盈已更新: %.4f", decision.Symbol, sideUpper, decision.TakeProfit)
				}
			}
			
			// 如果既没有提供止损也没有提供止盈，只是普通hold
			if decision.StopLoss == 0 && decision.TakeProfit == 0 {
				log.Printf("  ℹ️  %s 继续持仓，未调整止损止盈", decision.Symbol)
			}
			
			break
		}
	}
	
	if position == nil {
		log.Printf("  ⚠️  %s 未找到持仓，hold操作无效", decision.Symbol)
		// 不返回错误，因为可能是持仓已被平仓
	}
	
	return nil
}

// executeCloseShortWithRecord 执行平空仓并记录详细信息
func (at *AutoTrader) executeCloseShortWithRecord(decision *decision.Decision, actionRecord *logger.DecisionAction) error {
	log.Printf("  🔄 平空仓: %s", decision.Symbol)

	// 平仓
	order, err := at.trader.CloseShort(decision.Symbol, 0) // 0 = 全部平仓
	if err != nil {
		return err
	}

	// 记录订单ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// ⚠️ 关键：平仓后等待一小段时间，然后从持仓信息或市场价格获取实际平仓价格
	// 由于平仓后持仓已不存在，我们需要从市场价格或订单信息中获取
	time.Sleep(500 * time.Millisecond)
	
	// 尝试从订单返回结果中获取实际执行价格
	if execPrice, ok := order["executionPrice"].(float64); ok && execPrice > 0 {
		actionRecord.Price = execPrice
		log.Printf("  📊 实际平仓价格（从订单）: %.4f", execPrice)
	} else if avgPrice, ok := order["avgPrice"].(float64); ok && avgPrice > 0 {
		actionRecord.Price = avgPrice
		log.Printf("  📊 实际平仓价格（平均价格）: %.4f", avgPrice)
	} else {
		// 如果订单返回结果中没有价格，使用平仓后的市场价格作为备选
		// 注意：这可能不够准确，但比平仓前的价格更接近实际执行价格
		marketData, err := market.Get(decision.Symbol)
		if err == nil {
			actionRecord.Price = marketData.CurrentPrice
			log.Printf("  ⚠️ 无法获取实际平仓价格，使用平仓后市场价格: %.4f", marketData.CurrentPrice)
		} else {
			// 最后的备选：使用平仓前的市场价格
			marketData, _ := market.Get(decision.Symbol)
			if marketData != nil {
				actionRecord.Price = marketData.CurrentPrice
				log.Printf("  ⚠️ 使用市场价格作为备选: %.4f", marketData.CurrentPrice)
			}
		}
	}

	// 取消所有挂单
	if err := at.trader.CancelAllOrders(decision.Symbol); err != nil {
		log.Printf("  ⚠ 取消挂单失败: %v", err)
	}

	log.Printf("  ✓ 平仓成功，平仓价格: %.4f", actionRecord.Price)

	// ⚠️ 关键：更新平仓交易记录到本地文件
	if at.tradeLogger != nil {
		orderID := int64(0)
		if id, ok := order["orderId"].(int64); ok {
			orderID = id
		}
		_, err := at.tradeLogger.UpdateCloseTrade(
			decision.Symbol,
			"short",
			actionRecord.Price,
			actionRecord.Quantity,
			"ai_decision", // AI决策平仓
			orderID,
		)
		if err != nil {
			log.Printf("  ⚠️ 更新平仓交易记录失败: %v", err)
		} else {
			log.Printf("  ✅ 平仓交易已更新到本地文件")
		}
	}

	return nil
}

// GetID 获取trader ID
func (at *AutoTrader) GetID() string {
	return at.id
}

// GetName 获取trader名称
func (at *AutoTrader) GetName() string {
	return at.name
}

// GetAIModel 获取AI模型
func (at *AutoTrader) GetAIModel() string {
	return at.aiModel
}

// GetFuturesTrader 获取底层的FuturesTrader（如果交易所是币安）
// 用于需要直接访问币安API的功能（如获取交易历史）
func (at *AutoTrader) GetFuturesTrader() (*FuturesTrader, bool) {
	if at.exchange != "binance" {
		return nil, false
	}
	
	// 类型断言：trader字段应该是*FuturesTrader类型
	futuresTrader, ok := at.trader.(*FuturesTrader)
	return futuresTrader, ok
}

// GetExchange 获取交易所
func (at *AutoTrader) GetExchange() string {
	return at.exchange
}

// SetCustomPrompt 设置自定义交易策略prompt
func (at *AutoTrader) SetCustomPrompt(prompt string) {
	at.customPrompt = prompt
}

// SetOverrideBasePrompt 设置是否覆盖基础prompt
func (at *AutoTrader) SetOverrideBasePrompt(override bool) {
	at.overrideBasePrompt = override
}

// SetSystemPromptTemplate 设置系统提示词模板
func (at *AutoTrader) SetSystemPromptTemplate(templateName string) {
	at.systemPromptTemplate = templateName
}

// GetSystemPromptTemplate 获取当前系统提示词模板名称
func (at *AutoTrader) GetSystemPromptTemplate() string {
	return at.systemPromptTemplate
}

// GetDecisionLogger 获取决策日志记录器
func (at *AutoTrader) GetDecisionLogger() *logger.DecisionLogger {
	return at.decisionLogger
}

// GetTradeLogger 获取交易记录日志器
func (at *AutoTrader) GetTradeLogger() *logger.TradeLogger {
	return at.tradeLogger
}

// GetStatus 获取系统状态（用于API）
func (at *AutoTrader) GetStatus() map[string]interface{} {
	aiProvider := "DeepSeek"
	if at.config.UseQwen {
		aiProvider = "Qwen"
	}

	return map[string]interface{}{
		"trader_id":       at.id,
		"trader_name":     at.name,
		"ai_model":        at.aiModel,
		"exchange":        at.exchange,
		"is_running":      at.isRunning,
		"start_time":      at.startTime.Format(time.RFC3339),
		"runtime_minutes": int(time.Since(at.startTime).Minutes()),
		"call_count":      at.callCount,
		"initial_balance": at.initialBalance,
		"scan_interval":   at.config.ScanInterval.String(),
		"stop_until":      at.stopUntil.Format(time.RFC3339),
		"last_reset_time": at.lastResetTime.Format(time.RFC3339),
		"ai_provider":     aiProvider,
	}
}

// GetAccountInfo 获取账户信息（用于API）
func (at *AutoTrader) GetAccountInfo() (map[string]interface{}, error) {
	balance, err := at.trader.GetBalance()
	if err != nil {
		return nil, fmt.Errorf("获取余额失败: %w", err)
	}

	// 获取账户字段
	totalWalletBalance := 0.0
	totalUnrealizedProfit := 0.0
	availableBalance := 0.0

	if wallet, ok := balance["totalWalletBalance"].(float64); ok {
		totalWalletBalance = wallet
	}
	if unrealized, ok := balance["totalUnrealizedProfit"].(float64); ok {
		totalUnrealizedProfit = unrealized
	}
	if avail, ok := balance["availableBalance"].(float64); ok {
		availableBalance = avail
	}

	// Total Equity = 钱包余额 + 未实现盈亏
	totalEquity := totalWalletBalance + totalUnrealizedProfit

	// 获取持仓计算总保证金
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	totalMarginUsed := 0.0
	totalUnrealizedPnL := 0.0
	for _, pos := range positions {
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		totalUnrealizedPnL += unrealizedPnl

		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		marginUsed := (quantity * markPrice) / float64(leverage)
		totalMarginUsed += marginUsed
	}

	totalPnL := totalEquity - at.initialBalance
	totalPnLPct := 0.0
	if at.initialBalance > 0 {
		totalPnLPct = (totalPnL / at.initialBalance) * 100
	}

	marginUsedPct := 0.0
	if totalEquity > 0 {
		marginUsedPct = (totalMarginUsed / totalEquity) * 100
	}

	return map[string]interface{}{
		// 核心字段
		"total_equity":      totalEquity,           // 账户净值 = wallet + unrealized
		"wallet_balance":    totalWalletBalance,    // 钱包余额（不含未实现盈亏）
		"unrealized_profit": totalUnrealizedProfit, // 未实现盈亏（从API）
		"available_balance": availableBalance,      // 可用余额

		// 盈亏统计
		"total_pnl":            totalPnL,           // 总盈亏 = equity - initial
		"total_pnl_pct":        totalPnLPct,        // 总盈亏百分比
		"total_unrealized_pnl": totalUnrealizedPnL, // 未实现盈亏（从持仓计算）
		"initial_balance":      at.initialBalance,  // 初始余额
		"daily_pnl":            at.dailyPnL,        // 日盈亏

		// 持仓信息
		"position_count":  len(positions),  // 持仓数量
		"margin_used":     totalMarginUsed, // 保证金占用
		"margin_used_pct": marginUsedPct,   // 保证金使用率
	}, nil
}

// GetPositions 获取持仓列表（用于API）
func (at *AutoTrader) GetPositions() ([]map[string]interface{}, error) {
	positions, err := at.trader.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	var result []map[string]interface{}
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		entryPrice := pos["entryPrice"].(float64)
		markPrice := pos["markPrice"].(float64)
		quantity := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnl := pos["unRealizedProfit"].(float64)
		liquidationPrice := pos["liquidationPrice"].(float64)

		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}

		// 计算占用保证金
		marginUsed := (quantity * markPrice) / float64(leverage)

		// 计算盈亏百分比（基于保证金）
		// 收益率 = 未实现盈亏 / 保证金 × 100%
		pnlPct := 0.0
		if marginUsed > 0 {
			pnlPct = (unrealizedPnl / marginUsed) * 100
		}

		result = append(result, map[string]interface{}{
			"symbol":             symbol,
			"side":               side,
			"entry_price":        entryPrice,
			"mark_price":         markPrice,
			"quantity":           quantity,
			"leverage":           leverage,
			"unrealized_pnl":     unrealizedPnl,
			"unrealized_pnl_pct": pnlPct,
			"liquidation_price":  liquidationPrice,
			"margin_used":        marginUsed,
		})
	}

	return result, nil
}

// sortDecisionsByPriority 对决策排序：先平仓，再开仓，最后hold/wait
// 这样可以避免换仓时仓位叠加超限
func sortDecisionsByPriority(decisions []decision.Decision) []decision.Decision {
	if len(decisions) <= 1 {
		return decisions
	}

	// 定义优先级
	getActionPriority := func(action string) int {
		switch action {
		case "close_long", "close_short":
			return 1 // 最高优先级：先平仓
		case "open_long", "open_short":
			return 2 // 次优先级：后开仓
		case "hold", "wait":
			return 3 // 最低优先级：观望
		default:
			return 999 // 未知动作放最后
		}
	}

	// 复制决策列表
	sorted := make([]decision.Decision, len(decisions))
	copy(sorted, decisions)

	// 按优先级排序
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if getActionPriority(sorted[i].Action) > getActionPriority(sorted[j].Action) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// getCandidateCoins 获取交易员的候选币种列表
func (at *AutoTrader) getCandidateCoins() ([]decision.CandidateCoin, error) {
	if len(at.tradingCoins) == 0 {
		// 使用数据库配置的默认币种列表
		var candidateCoins []decision.CandidateCoin

		if len(at.defaultCoins) > 0 {
			// 使用数据库中配置的默认币种
			for _, coin := range at.defaultCoins {
				symbol := normalizeSymbol(coin)
				candidateCoins = append(candidateCoins, decision.CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"default"}, // 标记为数据库默认币种
				})
			}
			log.Printf("📋 [%s] 使用数据库默认币种: %d个币种 %v",
				at.name, len(candidateCoins), at.defaultCoins)
			return candidateCoins, nil
		} else {
			// 如果数据库中没有配置默认币种，则使用AI500+OI Top作为fallback
			const ai500Limit = 20 // AI500取前20个评分最高的币种

			mergedPool, err := pool.GetMergedCoinPool(ai500Limit)
			if err != nil {
				return nil, fmt.Errorf("获取合并币种池失败: %w", err)
			}

			// 构建候选币种列表（包含来源信息）
			for _, symbol := range mergedPool.AllSymbols {
				sources := mergedPool.SymbolSources[symbol]
				candidateCoins = append(candidateCoins, decision.CandidateCoin{
					Symbol:  symbol,
					Sources: sources, // "ai500" 和/或 "oi_top"
				})
			}

			log.Printf("📋 [%s] 数据库无默认币种配置，使用AI500+OI Top: AI500前%d + OI_Top20 = 总计%d个候选币种",
				at.name, ai500Limit, len(candidateCoins))
			return candidateCoins, nil
		}
	} else {
		// 使用自定义币种列表
		var candidateCoins []decision.CandidateCoin
		for _, coin := range at.tradingCoins {
			// 确保币种格式正确（转为大写USDT交易对）
			symbol := normalizeSymbol(coin)
			candidateCoins = append(candidateCoins, decision.CandidateCoin{
				Symbol:  symbol,
				Sources: []string{"custom"}, // 标记为自定义来源
			})
		}

		log.Printf("📋 [%s] 使用自定义币种: %d个币种 %v",
			at.name, len(candidateCoins), at.tradingCoins)
		return candidateCoins, nil
	}
}

// normalizeSymbol 标准化币种符号（确保以USDT结尾）
func normalizeSymbol(symbol string) string {
	// 转为大写
	symbol = strings.ToUpper(strings.TrimSpace(symbol))

	// 确保以USDT结尾
	if !strings.HasSuffix(symbol, "USDT") {
		symbol = symbol + "USDT"
	}

	return symbol
}

// detectAndRecordPositionChanges 检测持仓变化，自动记录止损/止盈触发的平仓
func (at *AutoTrader) detectAndRecordPositionChanges(record *logger.DecisionRecord) {
	// 获取当前持仓
	currentPositions, err := at.trader.GetPositions()
	if err != nil {
		log.Printf("⚠️  获取持仓失败，无法检测持仓变化: %v", err)
		return
	}

	// 构建当前持仓的key集合
	currentPositionKeys := make(map[string]bool)
	currentPositionMap := make(map[string]map[string]interface{})
	for _, pos := range currentPositions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		posKey := symbol + "_" + side
		currentPositionKeys[posKey] = true
		currentPositionMap[posKey] = pos
	}

	// 如果这是第一次运行（previousPositions为空），只保存当前持仓，不检测变化
	if at.previousPositions == nil {
		at.previousPositions = currentPositionMap
		return
	}

	// 检测消失的持仓（之前有，现在没有）
	for posKey, prevPos := range at.previousPositions {
		if !currentPositionKeys[posKey] {
			// 持仓消失了，可能是止损/止盈触发
			symbol := prevPos["symbol"].(string)
			side := prevPos["side"].(string)
			entryPrice := prevPos["entryPrice"].(float64)
			quantity := prevPos["positionAmt"].(float64)
			if quantity < 0 {
				quantity = -quantity
			}
			leverage := 10 // 默认值
			if lev, ok := prevPos["leverage"].(float64); ok {
				leverage = int(lev)
			}

			// 获取当前市场价格作为平仓价格
			marketData, err := market.Get(symbol)
			if err != nil {
				log.Printf("⚠️  获取 %s 市场价格失败: %v", symbol, err)
				continue
			}

			// 判断是止损还是止盈（基于盈亏）
			var action string
			if side == "long" {
				action = "close_long"
			} else {
				action = "close_short"
			}

			// 创建自动记录的平仓操作
			// ⚠️ 关键：尝试从历史记录中查找开仓信息，以便后续能正确匹配
			// 如果找不到，至少记录当前已知的信息（entryPrice, quantity, leverage）
			actionRecord := logger.DecisionAction{
				Action:    action,
				Symbol:    symbol,
				Quantity:  quantity,
				Leverage:  leverage,
				Price:     marketData.CurrentPrice,
				Timestamp: time.Now(),
				Success:   true,
			}
			
			// ⚠️ 关键修复：尝试从历史记录中查找对应的开仓记录，补充完整的开仓信息
			// 这样即使开仓记录在窗口外，也能正确匹配和计算盈亏
			if at.decisionLogger != nil {
				// 获取最近的历史记录，查找匹配的开仓记录
				// 使用更大的窗口（5000个周期）来查找开仓记录
				recentRecords, err := at.decisionLogger.GetLatestRecords(5000)
				if err == nil {
					// 从新到旧遍历，找到最近的开仓记录（在平仓时间之前）
					closeTime := actionRecord.Timestamp
					var foundOpenAction *logger.DecisionAction = nil
					
					for i := len(recentRecords) - 1; i >= 0; i-- {
						record := recentRecords[i]
						for _, prevAction := range record.Decisions {
							if !prevAction.Success {
								continue
							}
							// 查找匹配的开仓记录（必须在平仓时间之前）
							if (prevAction.Symbol == symbol) &&
								((side == "long" && prevAction.Action == "open_long") ||
									(side == "short" && prevAction.Action == "open_short")) &&
								prevAction.Timestamp.Before(closeTime) {
								// 如果还没有找到，或者这个开仓记录更接近平仓时间，则使用这个
								if foundOpenAction == nil || prevAction.Timestamp.After(foundOpenAction.Timestamp) {
									foundOpenAction = &prevAction
								}
							}
						}
					}
					
					if foundOpenAction != nil {
						// 找到匹配的开仓记录，更新平仓记录的开仓信息
						log.Printf("  📋 找到匹配的开仓记录: %s %s 开仓价 %.4f, 开仓时间: %s, 数量: %.4f, 杠杆: %dx",
							symbol, side, foundOpenAction.Price, foundOpenAction.Timestamp.Format("2006-01-02 15:04:05"),
							foundOpenAction.Quantity, foundOpenAction.Leverage)
						
						// ⚠️ 关键修复：使用找到的开仓记录的信息更新平仓记录
						// 这样 AnalyzePerformance 能够正确匹配开仓和平仓
						if foundOpenAction.Quantity > 0 {
							actionRecord.Quantity = foundOpenAction.Quantity
						}
						if foundOpenAction.Leverage > 0 {
							actionRecord.Leverage = foundOpenAction.Leverage
						}
						// 注意：actionRecord.Price 是平仓价格，不应该改
						// 但是 entryPrice 应该使用找到的开仓价格，而不是从 prevPos 中获取的
						// 因为从历史记录中找到的开仓价格更准确
						entryPrice = foundOpenAction.Price
					} else {
						log.Printf("  ⚠️  未找到匹配的开仓记录: %s %s (可能是手动开仓或开仓记录在窗口外)，使用持仓中的入场价: %.4f",
							symbol, side, entryPrice)
					}
				}
			}

			// ⚠️ 关键修复：记录到决策记录中，确保在AI决策之前记录
			// 这样 AnalyzePerformance 能够正确匹配开仓和平仓
			record.Decisions = append(record.Decisions, actionRecord)
			
			// 判断是止损还是止盈（基于盈亏）
			reason := "未知"
			if entryPrice > 0 && marketData.CurrentPrice > 0 {
				if side == "long" {
					if marketData.CurrentPrice < entryPrice {
						reason = "止损触发"
					} else {
						reason = "止盈触发"
					}
				} else {
					if marketData.CurrentPrice > entryPrice {
						reason = "止损触发"
					} else {
						reason = "止盈触发"
					}
				}
			}
			
			record.ExecutionLog = append(record.ExecutionLog, 
				fmt.Sprintf("🔔 自动检测到 %s %s 已平仓（%s），入场价: %.4f, 平仓价: %.4f", 
					symbol, side, reason, entryPrice, marketData.CurrentPrice))

			log.Printf("🔔 检测到持仓变化: %s %s 已平仓（%s），入场价: %.4f, 平仓价: %.4f, 数量: %.4f, 杠杆: %dx",
				symbol, side, reason, entryPrice, marketData.CurrentPrice, actionRecord.Quantity, actionRecord.Leverage)

			// 清理移动止损相关记录
			posKey := symbol + "_" + side
			delete(at.positionInitialStopLoss, posKey)
			delete(at.positionInitialTakeProfit, posKey)
			delete(at.positionInitialTakeProfit, posKey+"_dynamic") // 清理动态止盈记录
			delete(at.positionHighestProfit, posKey)
			delete(at.positionCurrentStopLoss, posKey)
			delete(at.positionHighestPrice, posKey)
			delete(at.positionLowestPrice, posKey)
		}
	}
	
	// 更新previousPositions为当前持仓状态（用于下次周期检测）
	at.previousPositions = currentPositionMap
}

// extractAutoClosedPositions 从决策记录中提取自动检测到的平仓信息
func (at *AutoTrader) extractAutoClosedPositions(record *logger.DecisionRecord) []decision.AutoClosedPosition {
	var autoClosed []decision.AutoClosedPosition
	
	// 遍历record.Decisions，找出自动检测到的平仓（在AI决策之前记录的）
	// 注意：这里假设在AI决策之前记录的平仓都是自动检测到的
	for _, action := range record.Decisions {
		if (action.Action == "close_long" || action.Action == "close_short") && action.Success {
			// 检查是否是自动检测到的平仓（通过ExecutionLog判断）
			isAutoClosed := false
			for _, logMsg := range record.ExecutionLog {
				if strings.Contains(logMsg, "🔔 自动检测到") && strings.Contains(logMsg, action.Symbol) {
					isAutoClosed = true
					break
				}
			}
			
			if isAutoClosed {
				side := "long"
				if action.Action == "close_short" {
					side = "short"
				}
				
				// 判断是止损还是止盈（基于盈亏，如果无法判断则标记为"未知"）
				reason := "未知"
				entryPrice := 0.0
				// 尝试从历史记录中查找开仓信息来判断盈亏
				if at.decisionLogger != nil {
					recentRecords, err := at.decisionLogger.GetLatestRecords(100)
					if err == nil {
						// 从新到旧遍历，找到匹配的开仓记录
						for i := len(recentRecords) - 1; i >= 0; i-- {
							rec := recentRecords[i]
							for _, prevAction := range rec.Decisions {
								if prevAction.Symbol == action.Symbol &&
									((side == "long" && prevAction.Action == "open_long") ||
										(side == "short" && prevAction.Action == "open_short")) {
									// 找到匹配的开仓记录，计算盈亏
									entryPrice = prevAction.Price
									if action.Price > 0 && prevAction.Price > 0 {
										if side == "long" {
											if action.Price < prevAction.Price {
												reason = "止损触发"
											} else {
												reason = "止盈触发"
											}
										} else {
											if action.Price > prevAction.Price {
												reason = "止损触发"
											} else {
												reason = "止盈触发"
											}
										}
									}
									break
								}
							}
							if reason != "未知" {
								break
							}
						}
					}
				}
				
				autoClosed = append(autoClosed, decision.AutoClosedPosition{
					Symbol:     action.Symbol,
					Side:       side,
					EntryPrice: entryPrice,
					ClosePrice: action.Price,
					Quantity:   action.Quantity,
					Leverage:   action.Leverage,
					Reason:     reason,
				})
			}
		}
	}
	
	return autoClosed
}

// updatePreviousPositions 更新上一个周期的持仓状态
func (at *AutoTrader) updatePreviousPositions() {
	positions, err := at.trader.GetPositions()
	if err != nil {
		log.Printf("⚠️  获取持仓失败，无法更新持仓状态: %v", err)
		return
	}

	at.previousPositions = make(map[string]map[string]interface{})
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		posKey := symbol + "_" + side
		at.previousPositions[posKey] = pos
	}
}

// checkAndAdjustTrailingStopLoss 检查并调整移动止损（自动执行，无需AI决策）
func (at *AutoTrader) checkAndAdjustTrailingStopLoss(record *logger.DecisionRecord) {
	positions, err := at.trader.GetPositions()
	if err != nil {
		log.Printf("⚠️  获取持仓失败，无法检查移动止损: %v", err)
		return
	}

	if len(positions) == 0 {
		return // 无持仓，无需检查
	}

	log.Println("🔍 开始自动移动止损检查...")

	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		posKey := symbol + "_" + side

		// 检查是否有初始止损记录（如果没有，说明是新持仓，跳过本次检查）
		initialStopLoss, hasInitialStopLoss := at.positionInitialStopLoss[posKey]
		if !hasInitialStopLoss {
			log.Printf("  ⏭️  %s %s 新持仓，跳过移动止损检查（等待下次周期）", symbol, side)
			continue // 新持仓，等待下次周期再检查
		}

		// ⚠️ 关键：检查持仓时间，避免刚开仓几分钟就平仓
		// 持仓时间必须至少15-30分钟，除非趋势真正反转或触发止损/止盈
		firstSeenTime, hasFirstSeenTime := at.positionFirstSeenTime[posKey]
		if hasFirstSeenTime {
			positionDuration := time.Now().UnixMilli() - firstSeenTime
			minHoldDuration := int64(15 * 60 * 1000) // 15分钟（毫秒），转换为int64
			if positionDuration < minHoldDuration {
				log.Printf("  ⏭️  %s %s 持仓时间过短（%.1f分钟 < 15分钟），跳过移动止损检查，避免过早平仓",
					symbol, side, float64(positionDuration)/(60*1000))
				continue // 持仓时间过短，跳过检查，避免过早平仓
			}
		}

		entryPrice, ok1 := pos["entryPrice"].(float64)
		markPrice, ok2 := pos["markPrice"].(float64)
		quantity, ok3 := pos["positionAmt"].(float64)
		if !ok1 || !ok2 || !ok3 || entryPrice <= 0 || markPrice <= 0 || quantity <= 0 {
			log.Printf("  ⚠️  %s %s 持仓数据不完整，跳过移动止损检查", symbol, side)
			continue
		}

		// 获取杠杆倍数（从持仓信息中获取）
		leverage := 10 // 默认值
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}

		// 计算当前盈利百分比（基于价格变化，与杠杆无关）
		// ⚠️ 重要：移动止损基于价格百分比计算，不是保证金百分比
		// 杠杆只影响实际盈亏金额，不影响价格百分比
		// 例如：入场价100，当前价102，盈利2%（无论杠杆是10x还是20x）
		var currentProfitPct float64
		if side == "long" {
			currentProfitPct = ((markPrice - entryPrice) / entryPrice) * 100
		} else {
			currentProfitPct = ((entryPrice - markPrice) / entryPrice) * 100
		}

		// ⚠️ 关键：计算相对于保证金的盈利百分比（考虑杠杆）
		// 用于判断是否达到盈利目标（目标价格涨幅 × 杠杆）
		var currentProfitPctWithLeverage float64
		if leverage > 0 {
			currentProfitPctWithLeverage = currentProfitPct * float64(leverage)
		} else {
			currentProfitPctWithLeverage = currentProfitPct // 如果杠杆未知，使用价格百分比
		}

		// ⚠️ 关键：计算相对于初始止盈目标的进度
		// 盈利达标 = (当前价格 - 入场价) / (止盈价 - 入场价) × 100%
		// 然后乘以杠杆，得到相对于保证金的盈利进度
		var progressToTarget float64 = 0.0
		var targetProfitPctWithLeverage float64 = 0.0
		initialTakeProfit, hasInitialTakeProfit := at.positionInitialTakeProfit[posKey]
		if hasInitialTakeProfit && initialTakeProfit > 0 {
			var targetDistance float64
			if side == "long" {
				targetDistance = ((initialTakeProfit - entryPrice) / entryPrice) * 100
				if targetDistance > 0 {
					progressToTarget = (currentProfitPct / targetDistance) * 100 // 当前盈利 / 目标盈利 × 100%
				}
			} else {
				targetDistance = ((entryPrice - initialTakeProfit) / entryPrice) * 100
				if targetDistance > 0 {
					progressToTarget = (currentProfitPct / targetDistance) * 100
				}
			}
			// 计算目标盈利（考虑杠杆）：目标价格涨幅 × 杠杆倍数
			if leverage > 0 {
				targetProfitPctWithLeverage = targetDistance * float64(leverage)
			}
		}

		// 记录盈利信息（用于日志和AI决策）
		log.Printf("  📊 %s %s 盈利分析: 价格盈利%.2f%%, 保证金盈利%.2f%% (杠杆%dx), 目标进度%.1f%%, 目标保证金盈利%.2f%%",
			symbol, side, currentProfitPct, currentProfitPctWithLeverage, leverage, progressToTarget, targetProfitPctWithLeverage)

		// 检测价格是否创新高/新低，并更新记录
		// ⚠️ 关键：每次价格创新高/新低时，重新计算并设置止损单和止盈单
		isNewHigh := false
		isNewLow := false
		
		// 初始化最高/最低价格记录（如果是新持仓）
		if at.positionHighestPrice[posKey] == 0 {
			at.positionHighestPrice[posKey] = markPrice
			at.positionLowestPrice[posKey] = markPrice
		}

		// 检测新高/新低
		if side == "long" {
			// 做多：检测是否创新高
			oldHighestPrice := at.positionHighestPrice[posKey]
			if markPrice > oldHighestPrice {
				isNewHigh = true
				at.positionHighestPrice[posKey] = markPrice
				log.Printf("  🚀 %s %s 价格创新高: %.4f (之前最高: %.4f)", symbol, side, markPrice, oldHighestPrice)
			}
			// 更新最低价格（用于回撤计算）
			if markPrice < at.positionLowestPrice[posKey] {
				at.positionLowestPrice[posKey] = markPrice
			}
		} else {
			// 做空：检测是否创新低
			oldLowestPrice := at.positionLowestPrice[posKey]
			if markPrice < oldLowestPrice {
				isNewLow = true
				at.positionLowestPrice[posKey] = markPrice
				log.Printf("  🚀 %s %s 价格创新低: %.4f (之前最低: %.4f)", symbol, side, markPrice, oldLowestPrice)
			}
			// 更新最高价格（用于回撤计算）
			if markPrice > at.positionHighestPrice[posKey] {
				at.positionHighestPrice[posKey] = markPrice
			}
		}

		// 更新最高盈利记录（价格百分比）
		// ⚠️ 关键：当价格创新高/新低时，同时更新最高盈利记录和动态止盈单
		oldHighestProfit := at.positionHighestProfit[posKey]
		if currentProfitPct > oldHighestProfit {
			at.positionHighestProfit[posKey] = currentProfitPct
			log.Printf("  📈 %s %s 更新最高盈利记录: %.2f%% (之前: %.2f%%)", symbol, side, currentProfitPct, oldHighestProfit)
			
			// ⚠️ 关键改进：在达到最高盈利点时立即更新止盈单，而不是等回撤后再设置
			// 这样可以在价格回撤时及时锁定利润
			if currentProfitPct > 0 {
				// 计算动态止盈价格：在当前价格基础上，给一点缓冲（0.2%），确保能成交
				var dynamicTakeProfitPrice float64
				if side == "long" {
					// 做多：止盈价 = 当前价 × 0.998，略低于当前价确保成交
					dynamicTakeProfitPrice = markPrice * 0.998
				} else {
					// 做空：止盈价 = 当前价 × 1.002，略高于当前价确保成交
					dynamicTakeProfitPrice = markPrice * 1.002
				}
				
				// 检查是否已经有动态止盈记录，避免频繁更新
				dynamicTPKey := posKey + "_dynamic"
				existingDynamicTP, hasDynamicTP := at.positionInitialTakeProfit[dynamicTPKey]
				
				// 如果新的动态止盈价优于现有价格（做多时更高，做空时更低），则更新
				shouldUpdate := false
				if !hasDynamicTP {
					shouldUpdate = true
				} else if side == "long" && dynamicTakeProfitPrice > existingDynamicTP {
					shouldUpdate = true
				} else if side == "short" && dynamicTakeProfitPrice < existingDynamicTP {
					shouldUpdate = true
				}
				
				if shouldUpdate {
					log.Printf("  🎯 %s %s 价格创新高/新低，更新动态止盈单至%.4f锁定利润（当前盈利%.2f%%）",
						symbol, side, dynamicTakeProfitPrice, currentProfitPct)
					
					// 更新止盈单
					if err := at.trader.SetTakeProfit(symbol, strings.ToUpper(side), quantity, dynamicTakeProfitPrice); err != nil {
						log.Printf("  ❌ %s %s 更新动态止盈单失败: %v", symbol, side, err)
						record.ExecutionLog = append(record.ExecutionLog,
							fmt.Sprintf("❌ %s %s 更新动态止盈单失败: %v", symbol, side, err))
					} else {
						// 记录动态止盈价格（用于下次检查，避免频繁更新）
						at.positionInitialTakeProfit[dynamicTPKey] = dynamicTakeProfitPrice
						log.Printf("  ✅ %s %s 动态止盈单已更新至%.4f（初始止盈目标: %.4f）",
							symbol, side, dynamicTakeProfitPrice, at.positionInitialTakeProfit[posKey])
						record.ExecutionLog = append(record.ExecutionLog,
							fmt.Sprintf("✅ %s %s 价格创新高/新低，动态止盈单已更新至%.4f（盈利%.2f%%）",
								symbol, side, dynamicTakeProfitPrice, currentProfitPct))
					}
				}
			}
		}

		// 计算止损距离（价格百分比，与杠杆无关）
		// 止损距离 = |入场价 - 初始止损价| / 入场价 × 100%
		var stopLossDistancePct float64
		if side == "long" {
			stopLossDistancePct = ((entryPrice - initialStopLoss) / entryPrice) * 100
		} else {
			stopLossDistancePct = ((initialStopLoss - entryPrice) / entryPrice) * 100
		}
		if stopLossDistancePct < 0 {
			stopLossDistancePct = -stopLossDistancePct
		}

		// 获取当前止损价格
		currentStopLoss := at.positionCurrentStopLoss[posKey]
		if currentStopLoss == 0 {
			currentStopLoss = initialStopLoss
		}

		// 注意：动态止盈逻辑已合并到价格创新高/新低的检查中（见上方）
		// 当价格创新高/新低时，会立即更新止盈单到当前价格附近，锁定利润

		// 检查回撤保护（调整止损，不直接止盈）
		// 回撤 = 最高盈利（价格百分比） - 当前盈利（价格百分比）
		// 回撤阈值 = 止损距离（价格百分比） × 1.5
		drawdownThreshold := stopLossDistancePct * 1.5 // 止损距离的1.5倍（价格百分比）
		
		// 获取最高盈利记录
		highestProfit := at.positionHighestProfit[posKey]
		
		// 计算回撤（从最高盈利点回撤的幅度）
		drawdown := highestProfit - currentProfitPct

		// ⚠️ 关键修复：只要曾经盈利过（highestProfit > 0），且回撤超过阈值，就触发保护
		// 即使当前已经亏损（currentProfitPct < 0），也应该触发保护
		if drawdown > drawdownThreshold && highestProfit > 0 {
			// 触发回撤保护
			var newStopLoss float64
			if currentProfitPct >= stopLossDistancePct*2 {
				// 当前盈利仍 ≥ 止损距离的2倍：调整至盈利的60%位置
				if side == "long" {
					newStopLoss = entryPrice + (markPrice-entryPrice)*0.6
				} else {
					newStopLoss = entryPrice - (entryPrice-markPrice)*0.6
				}
				log.Printf("  🛡️  %s %s 触发回撤保护（回撤%.2f%% > 阈值%.2f%%，最高盈利%.2f%%，当前盈利%.2f%%），调整止损至盈利60%%位置: %.4f",
					symbol, side, drawdown, drawdownThreshold, highestProfit, currentProfitPct, newStopLoss)
			} else if currentProfitPct >= stopLossDistancePct {
				// 当前盈利仍 ≥ 止损距离的1倍：调整至盈利的40%位置
				if side == "long" {
					newStopLoss = entryPrice + (markPrice-entryPrice)*0.4
				} else {
					newStopLoss = entryPrice - (entryPrice-markPrice)*0.4
				}
				log.Printf("  🛡️  %s %s 触发回撤保护（回撤%.2f%% > 阈值%.2f%%，最高盈利%.2f%%，当前盈利%.2f%%），调整止损至盈利40%%位置: %.4f",
					symbol, side, drawdown, drawdownThreshold, highestProfit, currentProfitPct, newStopLoss)
			} else if currentProfitPct >= 0 {
				// 当前盈利仍 ≥ 0但 < 止损距离的1倍：调整至保本
				newStopLoss = entryPrice
				log.Printf("  🛡️  %s %s 触发回撤保护（回撤%.2f%% > 阈值%.2f%%，最高盈利%.2f%%，当前盈利%.2f%%），调整止损至保本: %.4f",
					symbol, side, drawdown, drawdownThreshold, highestProfit, currentProfitPct, newStopLoss)
			} else {
				// 当前已经亏损：立即调整至保本或更保守位置（至少保本）
				// 如果当前止损已经低于保本，保持当前止损；否则调整至保本
				if (side == "long" && currentStopLoss < entryPrice) || (side == "short" && currentStopLoss > entryPrice) {
					newStopLoss = entryPrice
					log.Printf("  🚨 %s %s 触发回撤保护（回撤%.2f%% > 阈值%.2f%%，最高盈利%.2f%%，当前已亏损%.2f%%），立即调整止损至保本: %.4f",
						symbol, side, drawdown, drawdownThreshold, highestProfit, currentProfitPct, newStopLoss)
				} else {
					// 当前止损已经优于保本，保持不动
					log.Printf("  ⚠️  %s %s 触发回撤保护（回撤%.2f%% > 阈值%.2f%%，最高盈利%.2f%%，当前已亏损%.2f%%），但当前止损%.4f已优于保本%.4f，保持不动",
						symbol, side, drawdown, drawdownThreshold, highestProfit, currentProfitPct, currentStopLoss, entryPrice)
					continue // 跳过调整，但继续检查其他持仓
				}
			}

			// 检查新止损价是否优于当前止损价
			if (side == "long" && newStopLoss > currentStopLoss) || (side == "short" && newStopLoss < currentStopLoss) {
				if err := at.trader.SetStopLoss(symbol, strings.ToUpper(side), quantity, newStopLoss); err != nil {
					log.Printf("  ❌ %s %s 调整止损失败: %v", symbol, side, err)
					record.ExecutionLog = append(record.ExecutionLog,
						fmt.Sprintf("❌ %s %s 移动止损调整失败: %v", symbol, side, err))
				} else {
					at.positionCurrentStopLoss[posKey] = newStopLoss
					log.Printf("  ✅ %s %s 移动止损已调整: %.4f → %.4f", symbol, side, currentStopLoss, newStopLoss)
					record.ExecutionLog = append(record.ExecutionLog,
						fmt.Sprintf("✅ %s %s 移动止损已调整（回撤保护）: %.4f → %.4f", symbol, side, currentStopLoss, newStopLoss))
				}
			}
			continue // 回撤保护已处理，跳过常规移动止损检查
		}

		// ⚠️ 关键：如果价格创新高/新低，即使未达到阶段阈值，也要重新计算并设置止损
		// 这样可以确保止损单始终是最新的，避免止损单过期或未更新
		if isNewHigh || isNewLow {
			// 价格创新高/新低，重新计算止损位置
			var recalculatedStopLoss float64
			var recalcStage string
			var shouldRecalc bool

			// 根据当前盈利阶段重新计算止损
			if currentProfitPct >= stopLossDistancePct*6 {
				// 阶段4：锁定85%利润
				if side == "long" {
					recalculatedStopLoss = entryPrice + (markPrice-entryPrice)*0.85
				} else {
					recalculatedStopLoss = entryPrice - (entryPrice-markPrice)*0.85
				}
				recalcStage = "阶段4（锁定85%利润）"
				shouldRecalc = true
			} else if currentProfitPct >= stopLossDistancePct*5 {
				// 阶段4：锁定80%利润
				if side == "long" {
					recalculatedStopLoss = entryPrice + (markPrice-entryPrice)*0.80
				} else {
					recalculatedStopLoss = entryPrice - (entryPrice-markPrice)*0.80
				}
				recalcStage = "阶段4（锁定80%利润）"
				shouldRecalc = true
			} else if currentProfitPct >= stopLossDistancePct*4 {
				// 阶段4：锁定75%利润
				if side == "long" {
					recalculatedStopLoss = entryPrice + (markPrice-entryPrice)*0.75
				} else {
					recalculatedStopLoss = entryPrice - (entryPrice-markPrice)*0.75
				}
				recalcStage = "阶段4（锁定75%利润）"
				shouldRecalc = true
			} else if currentProfitPct >= stopLossDistancePct*3 {
				// 阶段3：锁定70%利润
				if side == "long" {
					recalculatedStopLoss = entryPrice + (markPrice-entryPrice)*0.70
				} else {
					recalculatedStopLoss = entryPrice - (entryPrice-markPrice)*0.70
				}
				recalcStage = "阶段3（锁定70%利润）"
				shouldRecalc = true
			} else if currentProfitPct >= stopLossDistancePct*2 {
				// 阶段2：锁定50%利润
				if side == "long" {
					recalculatedStopLoss = entryPrice + (markPrice-entryPrice)*0.50
				} else {
					recalculatedStopLoss = entryPrice - (entryPrice-markPrice)*0.50
				}
				recalcStage = "阶段2（锁定50%利润）"
				shouldRecalc = true
			} else if currentProfitPct >= stopLossDistancePct {
				// 阶段1：保本
				recalculatedStopLoss = entryPrice
				recalcStage = "阶段1（保本）"
				shouldRecalc = true
			}

			if shouldRecalc {
				// 检查新止损价是否优于当前止损价
				if (side == "long" && recalculatedStopLoss > currentStopLoss) || (side == "short" && recalculatedStopLoss < currentStopLoss) {
					// ⚠️ 关键：直接设置新的止损单，不取消所有订单
					// 交易所通常会自动替换旧的止损单，同时保留止盈单
					// 这样既能锁定利润（通过调整止损），又能保留初始止盈目标（让利润继续奔跑）
					if err := at.trader.SetStopLoss(symbol, strings.ToUpper(side), quantity, recalculatedStopLoss); err != nil {
						log.Printf("  ❌ %s %s 价格创新高/新低后重新设置止损失败: %v", symbol, side, err)
						record.ExecutionLog = append(record.ExecutionLog,
							fmt.Sprintf("❌ %s %s 价格创新高/新低后重新设置止损失败: %v", symbol, side, err))
					} else {
						at.positionCurrentStopLoss[posKey] = recalculatedStopLoss
						log.Printf("  🔄 %s %s 价格创新高/新低，重新设置止损 %s: 当前盈利%.2f%%, %.4f → %.4f (保留初始止盈目标: %.4f)",
							symbol, side, recalcStage, currentProfitPct, currentStopLoss, recalculatedStopLoss, at.positionInitialTakeProfit[posKey])
						record.ExecutionLog = append(record.ExecutionLog,
							fmt.Sprintf("🔄 %s %s 价格创新高/新低，重新设置止损 %s: %.4f → %.4f (保留初始止盈: %.4f)",
								symbol, side, recalcStage, currentStopLoss, recalculatedStopLoss, at.positionInitialTakeProfit[posKey]))
					}
					continue // 已处理，跳过常规移动止损检查
				} else {
					log.Printf("  ⚠️  %s %s 价格创新高/新低，但新止损价%.4f不优于当前止损价%.4f，保持不动",
						symbol, side, recalculatedStopLoss, currentStopLoss)
				}
			}
		}

		// 常规移动止损检查（基于盈利阶段，所有计算基于价格百分比）
		// ⚠️ 注意：这里的"盈利"是指价格变化百分比，不是保证金百分比
		// 例如：入场价100，当前价104，盈利4%（无论杠杆倍数）
		// ⚠️ 关键：移动止损只调整止损价格，不直接平仓
		// 只有在AI判断趋势反转时，才会通过close_long/close_short强制平仓
		var newStopLoss float64
		var stage string
		var shouldAdjust bool

		// ⚠️ 关键改进：盈利达标判断应该考虑杠杆和目标价格
		// 盈利达标 = (当前价格相对于目标价格的进度) × 杠杆倍数
		// 例如：目标盈利5%，当前进度80%，杠杆10x → 实际盈利 = 5% × 80% × 10 = 40%
		// 只有当实际盈利达到预期（如20%）且趋势反转时，才考虑平仓
		// 否则，只调整止损和止盈单，让利润继续奔跑

		if currentProfitPct >= stopLossDistancePct*6 {
			// 阶段4：锁定85%利润（基于价格变化）
			// 新止损价 = 入场价 + (当前价 - 入场价) × 0.85
			if side == "long" {
				newStopLoss = entryPrice + (markPrice-entryPrice)*0.85
			} else {
				newStopLoss = entryPrice - (entryPrice-markPrice)*0.85
			}
			stage = "阶段4（锁定85%利润）"
			shouldAdjust = true
		} else if currentProfitPct >= stopLossDistancePct*5 {
			// 阶段4：锁定80%利润（基于价格变化）
			if side == "long" {
				newStopLoss = entryPrice + (markPrice-entryPrice)*0.80
			} else {
				newStopLoss = entryPrice - (entryPrice-markPrice)*0.80
			}
			stage = "阶段4（锁定80%利润）"
			shouldAdjust = true
		} else if currentProfitPct >= stopLossDistancePct*4 {
			// 阶段4：锁定75%利润（基于价格变化）
			if side == "long" {
				newStopLoss = entryPrice + (markPrice-entryPrice)*0.75
			} else {
				newStopLoss = entryPrice - (entryPrice-markPrice)*0.75
			}
			stage = "阶段4（锁定75%利润）"
			shouldAdjust = true
		} else if currentProfitPct >= stopLossDistancePct*3 {
			// 阶段3：锁定70%利润（基于价格变化）
			if side == "long" {
				newStopLoss = entryPrice + (markPrice-entryPrice)*0.70
			} else {
				newStopLoss = entryPrice - (entryPrice-markPrice)*0.70
			}
			stage = "阶段3（锁定70%利润）"
			shouldAdjust = true
		} else if currentProfitPct >= stopLossDistancePct*2 {
			// 阶段2：锁定50%利润（基于价格变化）
			if side == "long" {
				newStopLoss = entryPrice + (markPrice-entryPrice)*0.50
			} else {
				newStopLoss = entryPrice - (entryPrice-markPrice)*0.50
			}
			stage = "阶段2（锁定50%利润）"
			shouldAdjust = true
		} else if currentProfitPct >= stopLossDistancePct {
			// 阶段1：保本（将止损调整至入场价）
			newStopLoss = entryPrice
			stage = "阶段1（保本）"
			shouldAdjust = true
		}

		if shouldAdjust {
			// 检查新止损价是否优于当前止损价（做多时只能上移，做空时只能下移）
			if (side == "long" && newStopLoss > currentStopLoss) || (side == "short" && newStopLoss < currentStopLoss) {
				// ⚠️ 关键：直接设置新的止损单，不取消所有订单
				// 交易所通常会自动替换旧的止损单，同时保留止盈单
				// 这样既能锁定利润（通过调整止损），又能保留初始止盈目标（让利润继续奔跑）
				if err := at.trader.SetStopLoss(symbol, strings.ToUpper(side), quantity, newStopLoss); err != nil {
					log.Printf("  ❌ %s %s 调整止损失败: %v", symbol, side, err)
					record.ExecutionLog = append(record.ExecutionLog,
						fmt.Sprintf("❌ %s %s 移动止损调整失败: %v", symbol, side, err))
				} else {
					at.positionCurrentStopLoss[posKey] = newStopLoss
					log.Printf("  ✅ %s %s 移动止损已调整 %s: 当前盈利%.2f%%, 止损距离%.2f%%, %.4f → %.4f (保留初始止盈目标: %.4f)",
						symbol, side, stage, currentProfitPct, stopLossDistancePct, currentStopLoss, newStopLoss, at.positionInitialTakeProfit[posKey])
					record.ExecutionLog = append(record.ExecutionLog,
						fmt.Sprintf("✅ %s %s 移动止损已调整 %s: 盈利%.2f%%, %.4f → %.4f (保留初始止盈: %.4f)",
							symbol, side, stage, currentProfitPct, currentStopLoss, newStopLoss, at.positionInitialTakeProfit[posKey]))
				}
			}
		}
	}

	// 清理已平仓的持仓记录
	at.cleanupClosedPositionRecords(positions)
}

// cleanupClosedPositionRecords 清理已平仓的持仓记录
func (at *AutoTrader) cleanupClosedPositionRecords(currentPositions []map[string]interface{}) {
	currentPosKeys := make(map[string]bool)
	for _, pos := range currentPositions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		posKey := symbol + "_" + side
		currentPosKeys[posKey] = true
	}

	// 清理已不存在的持仓记录
	for posKey := range at.positionInitialStopLoss {
		if !currentPosKeys[posKey] {
			delete(at.positionInitialStopLoss, posKey)
			delete(at.positionHighestProfit, posKey)
			delete(at.positionCurrentStopLoss, posKey)
			delete(at.positionHighestPrice, posKey)
			delete(at.positionLowestPrice, posKey)
		}
	}
}

// PositionScore 持仓评分（用于评估持仓质量）
type PositionScore struct {
	Symbol      string
	Side        string
	Quantity    float64
	EntryPrice  float64
	MarkPrice   float64
	UnrealizedPnL float64
	UnrealizedPnLPct float64
	MarginUsed  float64
	HoldingTime int64 // 持仓时长（毫秒）
	Score       float64 // 评分（越低越差，优先平仓）
}

// closeWorstPositionsToFreeMargin 平掉表现较差的仓位以释放保证金
// 返回释放的保证金数量
func (at *AutoTrader) closeWorstPositionsToFreeMargin(requiredMargin float64, excludeSymbol string) (float64, error) {
	positions, err := at.trader.GetPositions()
	if err != nil {
		return 0, fmt.Errorf("获取持仓失败: %w", err)
	}

	if len(positions) == 0 {
		return 0, fmt.Errorf("没有持仓可以平仓")
	}

	// 评估所有持仓的质量
	var positionScores []PositionScore
	now := time.Now().UnixMilli()

	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		
		// 排除要开仓的币种（避免平掉同币种）
		if symbol == excludeSymbol {
			continue
		}

		entryPrice, _ := pos["entryPrice"].(float64)
		markPrice, _ := pos["markPrice"].(float64)
		quantity, _ := pos["positionAmt"].(float64)
		if quantity < 0 {
			quantity = -quantity
		}
		unrealizedPnl, _ := pos["unRealizedProfit"].(float64)
		
		leverage := 10
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}

		// 计算盈亏百分比
		var pnlPct float64
		if side == "long" {
			pnlPct = ((markPrice - entryPrice) / entryPrice) * 100
		} else {
			pnlPct = ((entryPrice - markPrice) / entryPrice) * 100
		}

		// 计算占用保证金
		marginUsed := (quantity * markPrice) / float64(leverage)

		// 计算持仓时长
		posKey := symbol + "_" + side
		holdingTime := int64(0)
		if firstSeenTime, exists := at.positionFirstSeenTime[posKey]; exists {
			holdingTime = now - firstSeenTime
		}

		// 计算评分（越低越差，优先平仓）
		// 评分规则：
		// 1. 亏损的仓位优先平仓（亏损越多，评分越低）
		// 2. 盈利很少的仓位（<1%）优先平仓
		// 3. 持仓时间很长的亏损仓位优先平仓
		score := 100.0 // 初始高分

		if pnlPct < 0 {
			// 亏损仓位：亏损越多，评分越低
			score = pnlPct * 10 // 例如：-2% → -20分
		} else if pnlPct < 1.0 {
			// 盈利很少（<1%）：评分较低
			score = 10.0 - pnlPct*10 // 例如：0.5% → 5分
		} else {
			// 盈利较好的仓位：评分较高，不优先平仓
			score = 50.0 + pnlPct // 例如：5% → 55分
		}

		// 如果持仓时间很长（>2小时）且亏损或盈利很少，进一步降低评分
		if holdingTime > 2*3600*1000 { // 2小时 = 7200秒 = 7200000毫秒
			if pnlPct < 1.0 {
				score -= 20.0 // 长时间无进展，降低评分
			}
		}

		positionScores = append(positionScores, PositionScore{
			Symbol:          symbol,
			Side:            side,
			Quantity:        quantity,
			EntryPrice:      entryPrice,
			MarkPrice:       markPrice,
			UnrealizedPnL:   unrealizedPnl,
			UnrealizedPnLPct: pnlPct,
			MarginUsed:      marginUsed,
			HoldingTime:     holdingTime,
			Score:           score,
		})
	}

	if len(positionScores) == 0 {
		return 0, fmt.Errorf("没有可平仓的持仓（已排除 %s）", excludeSymbol)
	}

	// 按评分排序（评分越低越优先平仓）
	for i := 0; i < len(positionScores)-1; i++ {
		for j := i + 1; j < len(positionScores); j++ {
			if positionScores[i].Score > positionScores[j].Score {
				positionScores[i], positionScores[j] = positionScores[j], positionScores[i]
			}
		}
	}

	// 平掉表现最差的仓位，直到释放足够的保证金
	totalFreedMargin := 0.0
	closedCount := 0

	for _, posScore := range positionScores {
		if totalFreedMargin >= requiredMargin {
			break
		}

		log.Printf("  🔄 平掉表现较差的仓位: %s %s (盈亏: %.2f%%, 评分: %.2f, 保证金: %.2f USDT)", 
			posScore.Symbol, posScore.Side, posScore.UnrealizedPnLPct, posScore.Score, posScore.MarginUsed)

		var err error
		if posScore.Side == "long" {
			_, err = at.trader.CloseLong(posScore.Symbol, 0) // 0 = 全部平仓
		} else {
			_, err = at.trader.CloseShort(posScore.Symbol, 0)
		}

		if err != nil {
			log.Printf("  ❌ 平仓失败 %s %s: %v", posScore.Symbol, posScore.Side, err)
			continue
		}

		totalFreedMargin += posScore.MarginUsed
		closedCount++

		// 清理移动止损记录
		posKey := posScore.Symbol + "_" + posScore.Side
		delete(at.positionInitialStopLoss, posKey)
		delete(at.positionHighestProfit, posKey)
		delete(at.positionCurrentStopLoss, posKey)

		log.Printf("  ✅ 已平仓 %s %s，释放保证金: %.2f USDT", posScore.Symbol, posScore.Side, posScore.MarginUsed)
		
		// 等待一小段时间，确保订单成交
		time.Sleep(500 * time.Millisecond)
	}

	if totalFreedMargin < requiredMargin {
		return totalFreedMargin, fmt.Errorf("平掉了 %d 个仓位，释放保证金 %.2f USDT，但仍不足所需 %.2f USDT", 
			closedCount, totalFreedMargin, requiredMargin)
	}

        log.Printf("  ✅ 成功平掉 %d 个表现较差的仓位，释放保证金: %.2f USDT", closedCount, totalFreedMargin)                                                                                 
        return totalFreedMargin, nil
}

// syncBinanceTrades 同步币安交易记录（启动时调用）
func (at *AutoTrader) syncBinanceTrades() error {
	// 只支持币安交易所
	if at.exchange != "binance" {
		return nil
	}

	// 尝试将trader转换为FuturesTrader以调用GetUserTrades
	futuresTrader, ok := at.trader.(*FuturesTrader)
	if !ok {
		return fmt.Errorf("当前交易器不是币安期货交易器，无法同步交易记录")
	}

	// 获取所有交易过的币种（从本地记录或默认币种列表）
	symbols := make(map[string]bool)
	
	// 1. 从本地交易记录中获取所有交易过的币种
	localRecords, err := at.tradeLogger.GetTradeRecords("", 0) // 获取所有记录
	if err == nil {
		for _, record := range localRecords {
			if record.Symbol != "" {
				symbols[record.Symbol] = true
			}
		}
	}

	// 2. 添加默认币种列表
	for _, coin := range at.defaultCoins {
		if coin != "" {
			symbols[coin] = true
		}
	}

	// 3. 添加交易币种列表
	for _, coin := range at.tradingCoins {
		if coin != "" {
			symbols[coin] = true
		}
	}

	if len(symbols) == 0 {
		log.Printf("  ℹ️  没有找到需要同步的币种")
		return nil
	}

	log.Printf("  📋 开始同步 %d 个币种的交易记录...", len(symbols))

	// 获取最近7天的交易记录（币安API限制最多1000条）
	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)
	now := time.Now()

	syncedCount := 0
	updatedCount := 0
	createdCount := 0

	// 对每个币种获取交易记录
	for symbol := range symbols {
		// 获取该币种的交易记录
		binanceTrades, err := futuresTrader.GetUserTrades(symbol, 1000, &sevenDaysAgo, &now)
		if err != nil {
			log.Printf("  ⚠️  获取 %s 交易记录失败: %v", symbol, err)
			continue
		}

		if len(binanceTrades) == 0 {
			continue
		}

		// 按时间排序（从早到晚）
		sort.Slice(binanceTrades, func(i, j int) bool {
			timeI, _ := binanceTrades[i]["time"].(int64)
			timeJ, _ := binanceTrades[j]["time"].(int64)
			return timeI < timeJ
		})

		// 按持仓方向分组处理
		tradesByPosition := make(map[string][]map[string]interface{})
		for _, trade := range binanceTrades {
			positionSide, _ := trade["position_side"].(string)
			if positionSide == "" {
				continue
			}
			key := fmt.Sprintf("%s_%s", symbol, positionSide)
			tradesByPosition[key] = append(tradesByPosition[key], trade)
		}

		// 处理每个持仓方向的交易记录
		for posKey, trades := range tradesByPosition {
			// 解析持仓方向
			var side string
			if strings.Contains(posKey, "LONG") {
				side = "long"
			} else if strings.Contains(posKey, "SHORT") {
				side = "short"
			} else {
				continue
			}

			// 使用FIFO策略匹配开仓和平仓
			var openTrades []map[string]interface{} // 存储开仓记录（按时间顺序）
			
			for _, trade := range trades {
				tradeSide, _ := trade["side"].(string) // "BUY" or "SELL"
				positionSide, _ := trade["position_side"].(string)
				
				// 判断是开仓还是平仓
				isOpenTrade := false
				if positionSide == "LONG" && tradeSide == "BUY" {
					isOpenTrade = true
				} else if positionSide == "SHORT" && tradeSide == "SELL" {
					isOpenTrade = true
				}

				if isOpenTrade {
					// 开仓：添加到开仓列表
					openTrades = append(openTrades, trade)
				} else {
					// 平仓：匹配开仓记录
					closePrice, _ := trade["price"].(float64)
					closeQty, _ := trade["qty"].(float64)
					closeOrderID, _ := trade["order_id"].(int64)
					realizedPnl, _ := trade["realized_pnl"].(float64)

					// 使用FIFO策略匹配开仓记录
					remainingCloseQty := closeQty
					var matchedOpenTrades []map[string]interface{}
					
					for i := 0; i < len(openTrades) && remainingCloseQty > 0; i++ {
						openTrade := openTrades[i]
						openQty, _ := openTrade["qty"].(float64)
						
						// 检查这个开仓记录是否已经被完全匹配
						matchedQty, hasMatched := openTrade["matched_qty"].(float64)
						if !hasMatched {
							matchedQty = 0
						}
						
						remainingOpenQty := openQty - matchedQty
						if remainingOpenQty <= 0 {
							continue
						}

						// 匹配数量
						matched := math.Min(remainingOpenQty, remainingCloseQty)
						matchedOpenTrades = append(matchedOpenTrades, openTrade)
						
						// 更新已匹配数量
						openTrade["matched_qty"] = matchedQty + matched
						remainingCloseQty -= matched
					}

					// 如果有匹配的开仓记录，更新本地记录
					if len(matchedOpenTrades) > 0 && remainingCloseQty < closeQty {
						// 计算加权平均开仓价
						var totalOpenQty, totalOpenValue float64
						var openTime int64 = math.MaxInt64
						
						for _, openTrade := range matchedOpenTrades {
							openPrice, _ := openTrade["price"].(float64)
							openTimeVal, _ := openTrade["time"].(int64)
							
							matchedQty, _ := openTrade["matched_qty"].(float64)
							prevMatched, _ := openTrade["prev_matched_qty"].(float64)
							actualMatched := matchedQty - prevMatched
							
							if actualMatched > 0 {
								totalOpenQty += actualMatched
								totalOpenValue += openPrice * actualMatched
								if openTimeVal < openTime {
									openTime = openTimeVal
								}
							}
							openTrade["prev_matched_qty"] = matchedQty
						}

						if totalOpenQty > 0 {
							avgOpenPrice := totalOpenValue / totalOpenQty
							actualCloseQty := closeQty - remainingCloseQty
							
							// 检查本地是否已有对应的开仓记录
							localOpenRecords, err := at.tradeLogger.GetTradeRecords("open", 0)
							var localRecord *logger.TradeRecord
							if err == nil {
								// 查找匹配的未平仓记录（按时间顺序，最早的优先）
								for _, record := range localOpenRecords {
									if record.Symbol == symbol && record.Side == side {
										if localRecord == nil || record.OpenTime.Before(localRecord.OpenTime) {
											localRecord = record
										}
									}
								}
							}
							
							if localRecord != nil {
								// 如果本地记录的开仓时间与币安记录接近（5分钟内），认为是同一笔交易
								openTimeDiff := math.Abs(float64(localRecord.OpenTime.UnixMilli() - openTime))
								if openTimeDiff < 5*60*1000 { // 5分钟
									// 更新本地记录
									closeReason := "take_profit" // 默认认为是止盈
									if realizedPnl < 0 {
										closeReason = "stop_loss" // 如果亏损，认为是止损
									}
									
									_, err := at.tradeLogger.UpdateCloseTrade(symbol, side, closePrice, actualCloseQty, closeReason, closeOrderID)
									if err == nil {
										updatedCount++
										log.Printf("  ✓ 更新 %s %s 平仓记录（价格: %.4f, 数量: %.4f, 原因: %s）", 
											symbol, side, closePrice, actualCloseQty, closeReason)
									}
								} else {
									// 时间差异太大，可能是新的交易，创建新记录
									// 但这里我们暂时跳过，因为已经有本地记录了
								}
							} else {
								// 本地没有对应的开仓记录，可能是被动平单（止损/止盈触发）
								// 或者是在系统启动前就已经开仓的
								// 这种情况下，我们创建一个完整的交易记录（包括开仓和平仓）
								closeReason := "take_profit"
								if realizedPnl < 0 {
									closeReason = "stop_loss"
								}
								
								// 估算杠杆（从realized_pnl反推）
								estimatedLeverage := 15 // 默认15倍
								if avgOpenPrice > 0 && closePrice > 0 {
									priceChange := math.Abs(closePrice - avgOpenPrice) / avgOpenPrice
									if priceChange > 0 && realizedPnl != 0 {
										estimatedLeverage = int(math.Abs(realizedPnl / (priceChange * totalOpenQty * avgOpenPrice)))
										if estimatedLeverage < 1 || estimatedLeverage > 125 {
											estimatedLeverage = 15
										}
									}
								}
								
								// 创建完整的交易记录
								_, err := at.tradeLogger.RecordOpenTrade(symbol, side, avgOpenPrice, totalOpenQty, estimatedLeverage, 0, 0, 0)
								if err == nil {
									_, err = at.tradeLogger.UpdateCloseTrade(symbol, side, closePrice, actualCloseQty, closeReason, closeOrderID)
									if err == nil {
										createdCount++
										log.Printf("  ✓ 创建 %s %s 完整交易记录（开仓: %.4f, 平仓: %.4f, 原因: %s）", 
											symbol, side, avgOpenPrice, closePrice, closeReason)
									}
								}
							}
						}
					}
				}
			}
		}

		syncedCount++
	}

	log.Printf("  ✅ 同步完成：%d 个币种，更新 %d 条记录，创建 %d 条记录", syncedCount, updatedCount, createdCount)
	return nil
}
