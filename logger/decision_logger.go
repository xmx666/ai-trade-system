package logger

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DecisionRecord 决策记录
type DecisionRecord struct {
	Timestamp      time.Time          `json:"timestamp"`       // 决策时间
	CycleNumber    int                `json:"cycle_number"`    // 周期编号
	SystemPrompt   string             `json:"system_prompt"`   // 系统提示词（发送给AI的系统prompt）
	InputPrompt    string             `json:"input_prompt"`    // 发送给AI的输入prompt
	CoTTrace       string             `json:"cot_trace"`       // AI思维链（输出）
	DecisionJSON   string             `json:"decision_json"`   // 决策JSON
	AccountState   AccountSnapshot    `json:"account_state"`   // 账户状态快照
	Positions      []PositionSnapshot `json:"positions"`       // 持仓快照
	CandidateCoins []string           `json:"candidate_coins"` // 候选币种列表
	Decisions      []DecisionAction   `json:"decisions"`       // 执行的决策
	ExecutionLog   []string           `json:"execution_log"`   // 执行日志
	Success        bool               `json:"success"`         // 是否成功
	ErrorMessage   string             `json:"error_message"`    // 错误信息（如果有）
	FinishReason   string             `json:"finish_reason"`   // API返回的完成原因：stop（正常结束）、length（达到token限制）
}

// AccountSnapshot 账户状态快照
type AccountSnapshot struct {
	TotalBalance          float64 `json:"total_balance"`
	AvailableBalance      float64 `json:"available_balance"`
	TotalUnrealizedProfit float64 `json:"total_unrealized_profit"`
	PositionCount         int     `json:"position_count"`
	MarginUsedPct         float64 `json:"margin_used_pct"`
}

// PositionSnapshot 持仓快照
type PositionSnapshot struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"`
	PositionAmt      float64 `json:"position_amt"`
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	UnrealizedProfit float64 `json:"unrealized_profit"`
	Leverage         float64 `json:"leverage"`
	LiquidationPrice float64 `json:"liquidation_price"`
}

// DecisionAction 决策动作
type DecisionAction struct {
	Action    string    `json:"action"`    // open_long, open_short, close_long, close_short
	Symbol    string    `json:"symbol"`    // 币种
	Quantity  float64   `json:"quantity"`  // 数量
	Leverage  int       `json:"leverage"`  // 杠杆（开仓时）
	Price     float64   `json:"price"`     // 执行价格
	OrderID   int64     `json:"order_id"`  // 订单ID
	Timestamp time.Time `json:"timestamp"` // 执行时间
	Success   bool      `json:"success"`   // 是否成功
	Error     string    `json:"error"`     // 错误信息
}

// DecisionLogger 决策日志记录器
type DecisionLogger struct {
	logDir      string
	cycleNumber int
}

// NewDecisionLogger 创建决策日志记录器
func NewDecisionLogger(logDir string) *DecisionLogger {
	if logDir == "" {
		logDir = "decision_logs"
	}

	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("⚠ 创建日志目录失败: %v\n", err)
	}

	return &DecisionLogger{
		logDir:      logDir,
		cycleNumber: 0,
	}
}

// LogDecision 记录决策
func (l *DecisionLogger) LogDecision(record *DecisionRecord) error {
	l.cycleNumber++
	record.CycleNumber = l.cycleNumber
	record.Timestamp = time.Now()

	// 生成文件名：decision_YYYYMMDD_HHMMSS_cycleN.json
	filename := fmt.Sprintf("decision_%s_cycle%d.json",
		record.Timestamp.Format("20060102_150405"),
		record.CycleNumber)

	filepath := filepath.Join(l.logDir, filename)

	// 序列化为JSON（带缩进，方便阅读）
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化决策记录失败: %w", err)
	}

	// 写入文件
	if err := ioutil.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("写入决策记录失败: %w", err)
	}

	fmt.Printf("📝 决策记录已保存: %s\n", filename)
	return nil
}

// GetLatestRecords 获取最近N条记录（按时间正序：从旧到新）
func (l *DecisionLogger) GetLatestRecords(n int) ([]*DecisionRecord, error) {
	files, err := ioutil.ReadDir(l.logDir)
	if err != nil {
		return nil, fmt.Errorf("读取日志目录失败: %w", err)
	}

	// 先按修改时间倒序收集（最新的在前）
	var records []*DecisionRecord
	count := 0
	for i := len(files) - 1; i >= 0 && count < n; i-- {
		file := files[i]
		if file.IsDir() {
			continue
		}

		filepath := filepath.Join(l.logDir, file.Name())
		data, err := ioutil.ReadFile(filepath)
		if err != nil {
			continue
		}

		var record DecisionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		records = append(records, &record)
		count++
	}

	// 反转数组，让时间从旧到新排列（用于图表显示）
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	return records, nil
}

// GetRecordByDate 获取指定日期的所有记录
func (l *DecisionLogger) GetRecordByDate(date time.Time) ([]*DecisionRecord, error) {
	dateStr := date.Format("20060102")
	pattern := filepath.Join(l.logDir, fmt.Sprintf("decision_%s_*.json", dateStr))

	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("查找日志文件失败: %w", err)
	}

	var records []*DecisionRecord
	for _, filepath := range files {
		data, err := ioutil.ReadFile(filepath)
		if err != nil {
			continue
		}

		var record DecisionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		records = append(records, &record)
	}

	return records, nil
}

// CleanOldRecords 清理N天前的旧记录
func (l *DecisionLogger) CleanOldRecords(days int) error {
	cutoffTime := time.Now().AddDate(0, 0, -days)

	files, err := ioutil.ReadDir(l.logDir)
	if err != nil {
		return fmt.Errorf("读取日志目录失败: %w", err)
	}

	removedCount := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if file.ModTime().Before(cutoffTime) {
			filepath := filepath.Join(l.logDir, file.Name())
			if err := os.Remove(filepath); err != nil {
				fmt.Printf("⚠ 删除旧记录失败 %s: %v\n", file.Name(), err)
				continue
			}
			removedCount++
		}
	}

	if removedCount > 0 {
		fmt.Printf("🗑️ 已清理 %d 条旧记录（%d天前）\n", removedCount, days)
	}

	return nil
}

// GetStatistics 获取统计信息
func (l *DecisionLogger) GetStatistics() (*Statistics, error) {
	files, err := ioutil.ReadDir(l.logDir)
	if err != nil {
		return nil, fmt.Errorf("读取日志目录失败: %w", err)
	}

	stats := &Statistics{}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filepath := filepath.Join(l.logDir, file.Name())
		data, err := ioutil.ReadFile(filepath)
		if err != nil {
			continue
		}

		var record DecisionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		stats.TotalCycles++

		for _, action := range record.Decisions {
			if action.Success {
				switch action.Action {
				case "open_long", "open_short":
					stats.TotalOpenPositions++
				case "close_long", "close_short":
					stats.TotalClosePositions++
				}
			}
		}

		if record.Success {
			stats.SuccessfulCycles++
		} else {
			stats.FailedCycles++
		}
	}

	return stats, nil
}

// Statistics 统计信息
type Statistics struct {
	TotalCycles         int `json:"total_cycles"`
	SuccessfulCycles    int `json:"successful_cycles"`
	FailedCycles        int `json:"failed_cycles"`
	TotalOpenPositions  int `json:"total_open_positions"`
	TotalClosePositions int `json:"total_close_positions"`
}

// TradeOutcome 单笔交易结果
type TradeOutcome struct {
	Symbol        string    `json:"symbol"`         // 币种
	Side          string    `json:"side"`           // long/short
	Quantity      float64   `json:"quantity"`       // 仓位数量
	Leverage      int       `json:"leverage"`       // 杠杆倍数
	OpenPrice     float64   `json:"open_price"`     // 开仓价
	ClosePrice    float64   `json:"close_price"`    // 平仓价
	PositionValue float64   `json:"position_value"` // 仓位价值（quantity × openPrice）
	MarginUsed    float64   `json:"margin_used"`    // 保证金使用（positionValue / leverage）
	PnL           float64   `json:"pn_l"`           // 毛盈亏（USDT，未扣除手续费）
	PnLPct        float64   `json:"pn_l_pct"`       // 盈亏百分比（相对保证金）
	Fee           float64   `json:"fee"`            // 手续费（USDT，开仓0.04% + 平仓0.04% = 0.08%）
	NetPnL        float64   `json:"net_pn_l"`       // 净盈亏（USDT，扣除手续费后）
	NetPnLPct     float64   `json:"net_pn_l_pct"`  // 净盈亏百分比（相对保证金）
	Duration      string    `json:"duration"`       // 持仓时长
	OpenTime      time.Time `json:"open_time"`      // 开仓时间
	CloseTime     time.Time `json:"close_time"`     // 平仓时间
	WasStopLoss   bool      `json:"was_stop_loss"`  // 是否止损
}

// PerformanceAnalysis 交易表现分析
type PerformanceAnalysis struct {
	TotalTrades   int                           `json:"total_trades"`   // 总交易数
	WinningTrades int                           `json:"winning_trades"` // 盈利交易数
	LosingTrades  int                           `json:"losing_trades"`  // 亏损交易数
	WinRate       float64                       `json:"win_rate"`       // 胜率
	AvgWin        float64                       `json:"avg_win"`        // 平均盈利
	AvgLoss       float64                       `json:"avg_loss"`       // 平均亏损
	ProfitFactor  float64                       `json:"profit_factor"`  // 盈亏比
	SharpeRatio   float64                       `json:"sharpe_ratio"`   // 夏普比率（风险调整后收益）
	RecentTrades  []TradeOutcome                `json:"recent_trades"`  // 最近N笔交易
	SymbolStats   map[string]*SymbolPerformance `json:"symbol_stats"`   // 各币种表现
	BestSymbol    string                        `json:"best_symbol"`    // 表现最好的币种
	WorstSymbol   string                        `json:"worst_symbol"`   // 表现最差的币种
}

// SymbolPerformance 币种表现统计
type SymbolPerformance struct {
	Symbol        string  `json:"symbol"`         // 币种
	TotalTrades   int     `json:"total_trades"`   // 交易次数
	WinningTrades int     `json:"winning_trades"` // 盈利次数
	LosingTrades  int     `json:"losing_trades"`  // 亏损次数
	WinRate       float64 `json:"win_rate"`       // 胜率
	TotalPnL      float64 `json:"total_pn_l"`     // 总盈亏
	AvgPnL        float64 `json:"avg_pn_l"`       // 平均盈亏
}

// AnalyzePerformance 分析最近N个周期的交易表现
func (l *DecisionLogger) AnalyzePerformance(lookbackCycles int) (*PerformanceAnalysis, error) {
	records, err := l.GetLatestRecords(lookbackCycles)
	if err != nil {
		return nil, fmt.Errorf("读取历史记录失败: %w", err)
	}

	if len(records) == 0 {
		return &PerformanceAnalysis{
			RecentTrades: []TradeOutcome{},
			SymbolStats:  make(map[string]*SymbolPerformance),
		}, nil
	}

	analysis := &PerformanceAnalysis{
		RecentTrades: []TradeOutcome{},
		SymbolStats:  make(map[string]*SymbolPerformance),
	}

	// 追踪持仓状态：使用列表存储多个同方向的持仓（支持同一币种多次开仓）
	// key: symbol_side -> []{side, openPrice, openTime, quantity, leverage}
	openPositions := make(map[string][]map[string]interface{})

	// 为了避免开仓记录在窗口外导致匹配失败，需要先从所有历史记录中找出未平仓的持仓
	// 获取更多历史记录来构建完整的持仓状态（使用更大的窗口）
	allRecords, err := l.GetLatestRecords(lookbackCycles * 3) // 扩大3倍窗口
	if err == nil && len(allRecords) > len(records) {
		// 先从扩大的窗口中收集所有开仓记录
		for _, record := range allRecords {
			for _, action := range record.Decisions {
				if !action.Success {
					continue
				}

				symbol := action.Symbol
				side := ""
				if action.Action == "open_long" || action.Action == "close_long" {
					side = "long"
				} else if action.Action == "open_short" || action.Action == "close_short" {
					side = "short"
				}
				posKey := symbol + "_" + side

				switch action.Action {
				case "open_long", "open_short":
					// 记录开仓（添加到列表，支持多次开仓）
					if openPositions[posKey] == nil {
						openPositions[posKey] = make([]map[string]interface{}, 0)
					}
					openPositions[posKey] = append(openPositions[posKey], map[string]interface{}{
						"side":      side,
						"openPrice": action.Price,
						"openTime":  action.Timestamp,
						"quantity":  action.Quantity,
						"leverage":  action.Leverage,
					})
				case "close_long", "close_short":
					// 移除最早的开仓记录（FIFO：先进先出）
					if positions, exists := openPositions[posKey]; exists && len(positions) > 0 {
						// 移除第一个（最早的）开仓记录
						openPositions[posKey] = positions[1:]
						// 如果列表为空，删除key
						if len(openPositions[posKey]) == 0 {
							delete(openPositions, posKey)
						}
					}
				}
			}
		}
	}

	// 遍历分析窗口内的记录，生成交易结果
	for _, record := range records {
		for _, action := range record.Decisions {
			if !action.Success {
				continue
			}

			symbol := action.Symbol
			side := ""
			if action.Action == "open_long" || action.Action == "close_long" {
				side = "long"
			} else if action.Action == "open_short" || action.Action == "close_short" {
				side = "short"
			}
			posKey := symbol + "_" + side // 使用symbol_side作为key，区分多空持仓

			switch action.Action {
			case "open_long", "open_short":
				// 更新开仓记录（添加到列表，支持多次开仓）
				if openPositions[posKey] == nil {
					openPositions[posKey] = make([]map[string]interface{}, 0)
				}
				openPositions[posKey] = append(openPositions[posKey], map[string]interface{}{
					"side":      side,
					"openPrice": action.Price,
					"openTime":  action.Timestamp,
					"quantity":  action.Quantity,
					"leverage":  action.Leverage,
				})

			case "close_long", "close_short":
				// ⚠️ 关键修复：查找对应的开仓记录（可能来自预填充或当前窗口）
				// 使用FIFO策略：匹配最早的开仓记录
				// 但也要考虑时间顺序：开仓时间必须在平仓时间之前
				var matchedOpenPos map[string]interface{} = nil
				if positions, exists := openPositions[posKey]; exists && len(positions) > 0 {
					// 查找在平仓时间之前最近的开仓记录
					closeTime := action.Timestamp
					for _, pos := range positions {
						openTime := pos["openTime"].(time.Time)
						// 开仓时间必须在平仓时间之前
						if openTime.Before(closeTime) || openTime.Equal(closeTime) {
							// 如果还没有找到，或者这个开仓记录更接近平仓时间，则使用这个
							if matchedOpenPos == nil || openTime.After(matchedOpenPos["openTime"].(time.Time)) {
								matchedOpenPos = pos
							}
						}
					}
				}
				
				if matchedOpenPos != nil {
					// 获取匹配的开仓记录
					openPrice := matchedOpenPos["openPrice"].(float64)
					openTime := matchedOpenPos["openTime"].(time.Time)
					side := matchedOpenPos["side"].(string)
					quantity := matchedOpenPos["quantity"].(float64)
					leverage := matchedOpenPos["leverage"].(int)

						// 安全检查：确保杠杆至少为1，避免除以0或错误计算
						if leverage <= 0 {
							leverage = 1 // 如果杠杆为0或负数，使用默认值1
						}

					// 计算实际盈亏（USDT）
					// 合约交易 PnL 计算：quantity × 价格差
					// 注意：杠杆不影响绝对盈亏，只影响保证金需求
					var pnl float64
					if side == "long" {
						pnl = quantity * (action.Price - openPrice)
					} else {
						pnl = quantity * (openPrice - action.Price)
					}

						// 计算盈亏百分比（相对保证金，考虑杠杆）
						// ⚠️ 重要：这是保证金百分比，不是价格变化百分比
						// 例如：价格涨1%，10x杠杆 = 保证金涨10%
					positionValue := quantity * openPrice
					marginUsed := positionValue / float64(leverage)
					pnlPct := 0.0
					if marginUsed > 0 {
						pnlPct = (pnl / marginUsed) * 100
					}

					// 计算手续费（币安U本位合约：开仓0.04% + 平仓0.04% = 0.08%）
					// 手续费基于仓位价值（quantity × openPrice）计算
					fee := positionValue * 0.0008

					// 计算净盈亏（扣除手续费）
					netPnL := pnl - fee
					netPnLPct := 0.0
					if marginUsed > 0 {
						netPnLPct = (netPnL / marginUsed) * 100
					}

					// 记录交易结果
					outcome := TradeOutcome{
						Symbol:        symbol,
						Side:          side,
						Quantity:      quantity,
						Leverage:      leverage,
						OpenPrice:     openPrice,
						ClosePrice:    action.Price,
						PositionValue: positionValue,
						MarginUsed:    marginUsed,
						PnL:           pnl,           // 毛盈亏
						PnLPct:        pnlPct,        // 毛盈亏百分比
						Fee:           fee,           // 手续费
						NetPnL:        netPnL,        // 净盈亏（扣除手续费）
						NetPnLPct:     netPnLPct,     // 净盈亏百分比
						Duration:      action.Timestamp.Sub(openTime).String(),
						OpenTime:      openTime,
						CloseTime:     action.Timestamp,
					}

					analysis.RecentTrades = append(analysis.RecentTrades, outcome)
					analysis.TotalTrades++

					// 分类交易：盈利、亏损、持平（基于净盈亏，扣除手续费后）
					// 使用净盈亏（NetPnL）而不是毛盈亏（PnL）进行分类
					if netPnL > 0 {
						analysis.WinningTrades++
						analysis.AvgWin += netPnL  // 使用净盈利
					} else if netPnL < 0 {
						analysis.LosingTrades++
						analysis.AvgLoss += netPnL  // 使用净亏损
					}
					// netPnL == 0 的交易不计入盈利也不计入亏损，但计入总交易数

					// 更新币种统计
					if _, exists := analysis.SymbolStats[symbol]; !exists {
						analysis.SymbolStats[symbol] = &SymbolPerformance{
							Symbol: symbol,
						}
					}
					stats := analysis.SymbolStats[symbol]
					stats.TotalTrades++
					stats.TotalPnL += netPnL  // 使用净盈亏
					if netPnL > 0 {
						stats.WinningTrades++
					} else if netPnL < 0 {
						stats.LosingTrades++
					}

					// 移除已平仓记录（移除匹配的开仓记录）
					if positions, exists := openPositions[posKey]; exists {
						// 从列表中移除匹配的开仓记录
						newPositions := make([]map[string]interface{}, 0, len(positions))
						for _, pos := range positions {
							// 跳过匹配的开仓记录
							if pos["openTime"].(time.Time) != matchedOpenPos["openTime"].(time.Time) ||
								pos["openPrice"].(float64) != matchedOpenPos["openPrice"].(float64) {
								newPositions = append(newPositions, pos)
							}
						}
						openPositions[posKey] = newPositions
					// 如果列表为空，删除key
					if len(openPositions[posKey]) == 0 {
						delete(openPositions, posKey)
						}
					}
				} else {
					// ⚠️ 未找到匹配的开仓记录（可能开仓记录在窗口外、手动开仓、或系统重启后丢失）
					// 但为了确保所有平仓记录都能显示，我们尝试从币安API或历史记录中查找
					// 如果仍然找不到，至少创建一个不完整的交易记录（标记为未知开仓）
					
					// ⚠️ 关键修复：尝试从更大的历史窗口查找开仓记录
					// 使用时间顺序匹配：找到在平仓时间之前最近的开仓记录
					// 这对于止盈止损触发或手动平仓的情况非常重要
					extendedRecords, err := l.GetLatestRecords(lookbackCycles * 20) // 扩大20倍窗口（确保能找到）
					var foundOpenPos map[string]interface{} = nil
					if err == nil {
						// ⚠️ 关键：从新到旧遍历，找到在平仓时间之前最近的开仓记录
						// 这样可以确保找到最接近平仓时间的开仓记录
						closeTime := action.Timestamp
						for i := len(extendedRecords) - 1; i >= 0; i-- {
							extRecord := extendedRecords[i]
							for _, extAction := range extRecord.Decisions {
								if !extAction.Success {
									continue
								}
								extSymbol := extAction.Symbol
								extSide := ""
								if extAction.Action == "open_long" || extAction.Action == "close_long" {
									extSide = "long"
								} else if extAction.Action == "open_short" || extAction.Action == "close_short" {
									extSide = "short"
								}
								extPosKey := extSymbol + "_" + extSide
								
								// 如果找到匹配的开仓记录，且开仓时间在平仓时间之前
								if extPosKey == posKey && (extAction.Action == "open_long" || extAction.Action == "open_short") {
									if extAction.Timestamp.Before(closeTime) || extAction.Timestamp.Equal(closeTime) {
										// 如果还没有找到，或者这个开仓记录更接近平仓时间，则使用这个
										if foundOpenPos == nil || extAction.Timestamp.After(foundOpenPos["openTime"].(time.Time)) {
											foundOpenPos = map[string]interface{}{
												"side":      extSide,
												"openPrice": extAction.Price,
												"openTime":  extAction.Timestamp,
												"quantity":  extAction.Quantity,
												"leverage":  extAction.Leverage,
											}
										}
									}
								}
							}
						}
						if foundOpenPos != nil {
							fmt.Printf("✅ 在扩展窗口中找到匹配的开仓记录 - Symbol: %s, Side: %s, OpenPrice: %.4f, OpenTime: %s, CloseTime: %s\n",
								symbol, side, foundOpenPos["openPrice"].(float64), 
								foundOpenPos["openTime"].(time.Time).Format("2006-01-02 15:04:05"),
								action.Timestamp.Format("2006-01-02 15:04:05"))
						}
					}
					
					if foundOpenPos != nil {
						// 找到了开仓记录，使用它创建完整的交易记录
						openPrice := foundOpenPos["openPrice"].(float64)
						openTime := foundOpenPos["openTime"].(time.Time)
						side := foundOpenPos["side"].(string)
						quantity := foundOpenPos["quantity"].(float64)
						leverage := foundOpenPos["leverage"].(int)
						
						// 安全检查：确保杠杆至少为1，避免除以0或错误计算
						if leverage <= 0 {
							leverage = 1 // 如果杠杆为0或负数，使用默认值1
						}
						
						// 计算实际盈亏
						var pnl float64
						if side == "long" {
							pnl = quantity * (action.Price - openPrice)
						} else {
							pnl = quantity * (openPrice - action.Price)
						}
						
						// 计算盈亏百分比（相对保证金，考虑杠杆）
						// ⚠️ 重要：这是保证金百分比，不是价格变化百分比
						// 例如：价格涨1%，10x杠杆 = 保证金涨10%
						positionValue := quantity * openPrice
						marginUsed := positionValue / float64(leverage)
						pnlPct := 0.0
						if marginUsed > 0 {
							pnlPct = (pnl / marginUsed) * 100
						}
						
						// 计算手续费
						fee := positionValue * 0.0008
						netPnL := pnl - fee
						netPnLPct := 0.0
						if marginUsed > 0 {
							netPnLPct = (netPnL / marginUsed) * 100
						}
						
						// 创建交易记录
						outcome := TradeOutcome{
							Symbol:        symbol,
							Side:          side,
							Quantity:      quantity,
							Leverage:      leverage,
							OpenPrice:     openPrice,
							ClosePrice:    action.Price,
							PositionValue: positionValue,
							MarginUsed:    marginUsed,
							PnL:           pnl,
							PnLPct:        pnlPct,
							Fee:           fee,
							NetPnL:        netPnL,
							NetPnLPct:     netPnLPct,
							Duration:      action.Timestamp.Sub(openTime).String(),
							OpenTime:      openTime,
							CloseTime:     action.Timestamp,
							WasStopLoss:   true, // 标记为可能是止损/手动平仓
						}
						
						analysis.RecentTrades = append(analysis.RecentTrades, outcome)
						analysis.TotalTrades++
						
						// 分类交易
						if netPnL > 0 {
							analysis.WinningTrades++
							analysis.AvgWin += netPnL
						} else if netPnL < 0 {
							analysis.LosingTrades++
							analysis.AvgLoss += netPnL
						}
						
						// 更新币种统计
						if _, exists := analysis.SymbolStats[symbol]; !exists {
							analysis.SymbolStats[symbol] = &SymbolPerformance{
								Symbol: symbol,
							}
						}
						stats := analysis.SymbolStats[symbol]
						stats.TotalTrades++
						stats.TotalPnL += netPnL
						if netPnL > 0 {
							stats.WinningTrades++
						} else if netPnL < 0 {
							stats.LosingTrades++
						}
					} else {
						// ⚠️ 关键修复：仍然找不到开仓记录
						// 这可能是手动平仓、止损触发或开仓记录在更早的历史中
						fmt.Printf("⚠️  警告: 平仓记录未找到匹配的开仓记录 - Symbol: %s, Side: %s, ClosePrice: %.4f, Time: %s\n",
							symbol, side, action.Price, action.Timestamp.Format("2006-01-02 15:04:05"))
						fmt.Printf("  💡 提示: 这可能是手动平仓、止损触发或开仓记录在更早的历史中\n")
						fmt.Printf("  ⚠️  注意: 此平仓记录无法计算盈亏，请从币安查看完整交易信息\n")
						// 注意：不创建不完整的交易记录，因为缺少开仓信息无法准确计算盈亏
						// 用户应该从币安API查看完整信息（通过 /api/binance-trades 接口）
					}
				}
			}
		}
	}

	// 检查是否有未平仓的持仓（可能还在持仓中，或平仓记录丢失）
	if len(openPositions) > 0 {
		totalUnmatched := 0
		for _, positions := range openPositions {
			totalUnmatched += len(positions)
		}
		fmt.Printf("⚠️  警告: 发现 %d 个未匹配的开仓记录（可能还在持仓中，或平仓记录丢失）\n", totalUnmatched)
		for posKey, positions := range openPositions {
			if len(positions) == 0 {
				continue
			}
			symbol := strings.Split(posKey, "_")[0]
			side := strings.Split(posKey, "_")[1]
			// 显示第一个未匹配的开仓记录
			openPos := positions[0]
			fmt.Printf("  - %s %s: 开仓价 %.4f, 开仓时间: %s, 数量: %.4f, 杠杆: %dx (还有 %d 个未匹配)\n",
				symbol, side, openPos["openPrice"].(float64), openPos["openTime"].(time.Time).Format("2006-01-02 15:04:05"),
				openPos["quantity"].(float64), openPos["leverage"].(int), len(positions)-1)
		}
	}

	// 计算统计指标
	if analysis.TotalTrades > 0 {
		analysis.WinRate = (float64(analysis.WinningTrades) / float64(analysis.TotalTrades)) * 100

		// 计算总盈利和总亏损
		totalWinAmount := analysis.AvgWin   // 当前是累加的总和
		totalLossAmount := analysis.AvgLoss // 当前是累加的总和（负数）

		if analysis.WinningTrades > 0 {
			analysis.AvgWin /= float64(analysis.WinningTrades)
		}
		if analysis.LosingTrades > 0 {
			analysis.AvgLoss /= float64(analysis.LosingTrades)
		}

		// Profit Factor = 总盈利 / 总亏损（绝对值）
		// 注意：totalLossAmount 是负数，所以取负号得到绝对值
		if totalLossAmount != 0 {
			analysis.ProfitFactor = totalWinAmount / (-totalLossAmount)
		} else if totalWinAmount > 0 {
			// 只有盈利没有亏损的情况，设置为一个很大的值表示完美策略
			analysis.ProfitFactor = 999.0
		}
	}

	// 计算各币种胜率和平均盈亏
	bestPnL := -999999.0
	worstPnL := 999999.0
	for symbol, stats := range analysis.SymbolStats {
		if stats.TotalTrades > 0 {
			stats.WinRate = (float64(stats.WinningTrades) / float64(stats.TotalTrades)) * 100
			stats.AvgPnL = stats.TotalPnL / float64(stats.TotalTrades)

			if stats.TotalPnL > bestPnL {
				bestPnL = stats.TotalPnL
				analysis.BestSymbol = symbol
			}
			if stats.TotalPnL < worstPnL {
				worstPnL = stats.TotalPnL
				analysis.WorstSymbol = symbol
			}
		}
	}

	// 反转数组，让最新的交易在前（保留所有历史交易）
	if len(analysis.RecentTrades) > 0 {
		for i, j := 0, len(analysis.RecentTrades)-1; i < j; i, j = i+1, j-1 {
			analysis.RecentTrades[i], analysis.RecentTrades[j] = analysis.RecentTrades[j], analysis.RecentTrades[i]
		}
	}

	// 计算夏普比率（需要至少2个数据点）
	analysis.SharpeRatio = l.calculateSharpeRatio(records)

	return analysis, nil
}

// calculateSharpeRatio 计算夏普比率
// 基于账户净值的变化计算风险调整后收益
func (l *DecisionLogger) calculateSharpeRatio(records []*DecisionRecord) float64 {
	if len(records) < 2 {
		return 0.0
	}

	// 提取每个周期的账户净值
	// 注意：TotalBalance字段实际存储的是TotalEquity（账户总净值）
	// TotalUnrealizedProfit字段实际存储的是TotalPnL（相对初始余额的盈亏）
	var equities []float64
	for _, record := range records {
		// 直接使用TotalBalance，因为它已经是完整的账户净值
		equity := record.AccountState.TotalBalance
		if equity > 0 {
			equities = append(equities, equity)
		}
	}

	if len(equities) < 2 {
		return 0.0
	}

	// 计算周期收益率（period returns）
	var returns []float64
	for i := 1; i < len(equities); i++ {
		if equities[i-1] > 0 {
			periodReturn := (equities[i] - equities[i-1]) / equities[i-1]
			returns = append(returns, periodReturn)
		}
	}

	if len(returns) == 0 {
		return 0.0
	}

	// 计算平均收益率
	sumReturns := 0.0
	for _, r := range returns {
		sumReturns += r
	}
	meanReturn := sumReturns / float64(len(returns))

	// 计算收益率标准差
	sumSquaredDiff := 0.0
	for _, r := range returns {
		diff := r - meanReturn
		sumSquaredDiff += diff * diff
	}
	variance := sumSquaredDiff / float64(len(returns))
	stdDev := math.Sqrt(variance)

	// 避免除以零
	if stdDev == 0 {
		if meanReturn > 0 {
			return 999.0 // 无波动的正收益
		} else if meanReturn < 0 {
			return -999.0 // 无波动的负收益
		}
		return 0.0
	}

	// 计算夏普比率（假设无风险利率为0）
	// 注：直接返回周期级别的夏普比率（非年化），正常范围 -2 到 +2
	sharpeRatio := meanReturn / stdDev
	return sharpeRatio
}

// GetTodayTradeCount 获取今天的交易次数（开仓+平仓）
// 注意：保留此函数以保持向后兼容，但建议使用 GetHourlyTradeCount
func (l *DecisionLogger) GetTodayTradeCount() (int, error) {
	return l.GetHourlyTradeCount()
}

// GetHourlyTradeCount 获取最近1小时的交易次数（一次开单+一次平单算一个操作）
func (l *DecisionLogger) GetHourlyTradeCount() (int, error) {
	// 获取最近1小时的时间范围
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)

	files, err := ioutil.ReadDir(l.logDir)
	if err != nil {
		return 0, fmt.Errorf("读取日志目录失败: %w", err)
	}

	// 收集最近1小时内的所有开仓和平仓操作
	type TradeAction struct {
		Action    string
		Symbol    string
		Timestamp time.Time
	}

	var opens []TradeAction   // 开仓操作
	var closes []TradeAction  // 平仓操作

	// 遍历所有日志文件，收集最近1小时内的交易操作
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filepath := filepath.Join(l.logDir, file.Name())
		data, err := ioutil.ReadFile(filepath)
		if err != nil {
			continue
		}

		var record DecisionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		// 收集成功的开仓和平仓操作
		for _, action := range record.Decisions {
			if action.Success {
				// 检查交易时间是否在最近1小时内
				if action.Timestamp.After(oneHourAgo) && action.Timestamp.Before(now) {
					if action.Action == "open_long" || action.Action == "open_short" {
						opens = append(opens, TradeAction{
							Action:    action.Action,
							Symbol:    action.Symbol,
							Timestamp: action.Timestamp,
						})
					} else if action.Action == "close_long" || action.Action == "close_short" {
						closes = append(closes, TradeAction{
							Action:    action.Action,
							Symbol:    action.Symbol,
							Timestamp: action.Timestamp,
						})
					}
				}
			}
		}
	}

	// 配对开仓和平仓：一次开单+一次平单算一个操作
	// 配对规则：同一个symbol，同一个方向（long/short），开仓时间早于平仓时间
	tradeCount := 0
	matchedCloses := make(map[int]bool) // 标记已配对的平仓

	for _, open := range opens {
		// 确定对应的平仓操作类型
		var closeAction string
		if open.Action == "open_long" {
			closeAction = "close_long"
		} else if open.Action == "open_short" {
			closeAction = "close_short"
		}

		// 查找匹配的平仓操作（同一个symbol，同一个方向，时间在开仓之后）
		matched := false
		for j, close := range closes {
			if matchedCloses[j] {
				continue // 已配对，跳过
			}
			if close.Symbol == open.Symbol && close.Action == closeAction && close.Timestamp.After(open.Timestamp) {
				// 找到匹配的平仓，配对成功，算一次操作
				matchedCloses[j] = true
				matched = true
				tradeCount++
				break
			}
		}

		// 如果没有找到匹配的平仓，开仓单独算一次操作（这种情况应该很少，可能是刚开仓还未平仓）
		if !matched {
			tradeCount++
		}
	}

	// 统计未配对的平仓操作（这种情况应该很少，可能是平仓了之前开的仓）
	for j := range closes {
		if !matchedCloses[j] {
			tradeCount++
		}
	}

	return tradeCount, nil
}

// GetLastOpenTradeTime 获取最近一次开单的时间（返回距离现在的小时数，如果没有开单记录则返回-1）
func (l *DecisionLogger) GetLastOpenTradeTime() (float64, error) {
	files, err := ioutil.ReadDir(l.logDir)
	if err != nil {
		return -1, fmt.Errorf("读取日志目录失败: %w", err)
	}

	var lastOpenTime time.Time
	found := false

	// 遍历所有日志文件，查找最近一次开单操作
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filepath := filepath.Join(l.logDir, file.Name())
		data, err := ioutil.ReadFile(filepath)
		if err != nil {
			continue
		}

		var record DecisionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		// 查找成功的开仓操作
		for _, action := range record.Decisions {
			if action.Success && (action.Action == "open_long" || action.Action == "open_short") {
				if !found || action.Timestamp.After(lastOpenTime) {
					lastOpenTime = action.Timestamp
					found = true
				}
			}
		}
	}

	if !found {
		return -1, nil // 没有找到开单记录
	}

	// 计算距离现在的小时数
	hoursSinceLastTrade := time.Since(lastOpenTime).Hours()
	return hoursSinceLastTrade, nil
}
