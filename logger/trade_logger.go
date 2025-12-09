package logger

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TradeRecord 交易记录（完整的开仓和平仓信息）
type TradeRecord struct {
	// 交易ID（唯一标识）
	TradeID string `json:"trade_id"`

	// 币种和方向
	Symbol string `json:"symbol"`
	Side   string `json:"side"` // "long" or "short"

	// 开仓信息
	OpenPrice     float64   `json:"open_price"`
	OpenQuantity  float64   `json:"open_quantity"`
	OpenLeverage  int       `json:"open_leverage"`
	OpenTime      time.Time `json:"open_time"`
	OpenOrderID   int64     `json:"open_order_id,omitempty"`

	// 平仓信息（如果已平仓）
	ClosePrice     float64    `json:"close_price,omitempty"`
	CloseQuantity  float64    `json:"close_quantity,omitempty"`
	CloseTime      *time.Time `json:"close_time,omitempty"`
	CloseOrderID   int64      `json:"close_order_id,omitempty"`
	CloseReason    string     `json:"close_reason,omitempty"` // "ai_decision", "stop_loss", "take_profit", "manual", "trailing_stop"

	// 盈亏信息（如果已平仓）
	PnL           float64 `json:"pnl,omitempty"`            // 毛盈亏（USDT）
	PnLPct        float64 `json:"pnl_pct,omitempty"`        // 毛盈亏百分比（价格百分比）
	PnLWithLeverage float64 `json:"pnl_with_leverage,omitempty"` // 保证金盈亏百分比（价格百分比 × 杠杆）
	Fee            float64 `json:"fee,omitempty"`            // 手续费（USDT）
	NetPnL         float64 `json:"net_pnl,omitempty"`        // 净盈亏（USDT）
	NetPnLPct      float64 `json:"net_pnl_pct,omitempty"`    // 净盈亏百分比（保证金百分比）

	// 持仓时长（如果已平仓）
	Duration string `json:"duration,omitempty"` // 例如："2h30m15s"

	// 止损止盈信息
	InitialStopLoss   float64 `json:"initial_stop_loss,omitempty"`
	InitialTakeProfit float64 `json:"initial_take_profit,omitempty"`
	FinalStopLoss     float64 `json:"final_stop_loss,omitempty"` // 平仓时的止损价格

	// 状态
	Status string `json:"status"` // "open" or "closed"

	// 更新时间
	UpdatedAt time.Time `json:"updated_at"`
}

// TradeLogger 交易记录日志器
type TradeLogger struct {
	tradeDir string        // 交易记录目录
	mutex    sync.RWMutex // 读写锁，保护并发访问
}

// NewTradeLogger 创建交易记录日志器
func NewTradeLogger(tradeDir string) *TradeLogger {
	if tradeDir == "" {
		tradeDir = "trade_logs"
	}

	// 确保目录存在
	if err := os.MkdirAll(tradeDir, 0755); err != nil {
		fmt.Printf("⚠️ 创建交易记录目录失败: %v\n", err)
	}

	return &TradeLogger{
		tradeDir: tradeDir,
	}
}

// generateTradeID 生成交易ID（基于时间戳和随机数）
func generateTradeID(symbol, side string) string {
	return fmt.Sprintf("%s_%s_%d", symbol, side, time.Now().UnixMilli())
}

// RecordOpenTrade 记录开仓交易
func (tl *TradeLogger) RecordOpenTrade(symbol, side string, price, quantity float64, leverage int, orderID int64, stopLoss, takeProfit float64) (*TradeRecord, error) {
	tl.mutex.Lock()
	defer tl.mutex.Unlock()

	tradeID := generateTradeID(symbol, side)
	now := time.Now()

	record := &TradeRecord{
		TradeID:          tradeID,
		Symbol:           symbol,
		Side:             side,
		OpenPrice:        price,
		OpenQuantity:     quantity,
		OpenLeverage:     leverage,
		OpenTime:         now,
		OpenOrderID:      orderID,
		InitialStopLoss:  stopLoss,
		InitialTakeProfit: takeProfit,
		Status:           "open",
		UpdatedAt:        now,
	}

	// 保存到文件
	if err := tl.saveTradeRecord(record); err != nil {
		return nil, fmt.Errorf("保存开仓记录失败: %w", err)
	}

	return record, nil
}

// UpdateCloseTrade 更新平仓交易
func (tl *TradeLogger) UpdateCloseTrade(symbol, side string, closePrice, closeQuantity float64, closeReason string, orderID int64) (*TradeRecord, error) {
	tl.mutex.Lock()
	defer tl.mutex.Unlock()

	// 查找对应的开仓记录
	record, err := tl.findOpenTrade(symbol, side)
	if err != nil {
		return nil, fmt.Errorf("查找开仓记录失败: %w", err)
	}

	if record == nil {
		return nil, fmt.Errorf("未找到对应的开仓记录: %s %s", symbol, side)
	}

	// 更新平仓信息
	now := time.Now()
	record.ClosePrice = closePrice
	record.CloseQuantity = closeQuantity
	record.CloseTime = &now
	record.CloseOrderID = orderID
	record.CloseReason = closeReason
	record.Status = "closed"
	record.UpdatedAt = now

	// 计算持仓时长
	if record.CloseTime != nil {
		record.Duration = record.CloseTime.Sub(record.OpenTime).String()
	}

	// 计算盈亏
	tl.calculatePnL(record)

	// 保存更新后的记录
	if err := tl.saveTradeRecord(record); err != nil {
		return nil, fmt.Errorf("保存平仓记录失败: %w", err)
	}

	return record, nil
}

// calculatePnL 计算盈亏
func (tl *TradeLogger) calculatePnL(record *TradeRecord) {
	if record.ClosePrice == 0 || record.CloseTime == nil {
		return
	}

	// 计算毛盈亏（USDT）
	var pnl float64
	if record.Side == "long" {
		pnl = record.OpenQuantity * (record.ClosePrice - record.OpenPrice)
	} else {
		pnl = record.OpenQuantity * (record.OpenPrice - record.ClosePrice)
	}
	record.PnL = pnl

	// 计算毛盈亏百分比（价格百分比）
	record.PnLPct = ((record.ClosePrice - record.OpenPrice) / record.OpenPrice) * 100
	if record.Side == "short" {
		record.PnLPct = -record.PnLPct
	}

	// 计算保证金盈亏百分比（价格百分比 × 杠杆）
	if record.OpenLeverage > 0 {
		record.PnLWithLeverage = record.PnLPct * float64(record.OpenLeverage)
	}

	// 计算手续费（开仓 + 平仓）
	positionValue := record.OpenQuantity * record.OpenPrice
	record.Fee = positionValue * 0.0008 // 0.04% * 2 = 0.08%

	// 计算净盈亏
	record.NetPnL = pnl - record.Fee

	// 计算净盈亏百分比（保证金百分比）
	if record.OpenLeverage > 0 {
		marginUsed := positionValue / float64(record.OpenLeverage)
		if marginUsed > 0 {
			record.NetPnLPct = (record.NetPnL / marginUsed) * 100
		}
	}
}

// findOpenTrade 查找未平仓的交易记录（FIFO：先进先出）
func (tl *TradeLogger) findOpenTrade(symbol, side string) (*TradeRecord, error) {
	// 读取所有交易记录
	records, err := tl.getAllTradeRecords()
	if err != nil {
		return nil, err
	}

	// 查找匹配的未平仓记录（按时间顺序，最早的优先）
	var oldestOpenTrade *TradeRecord
	for _, record := range records {
		if record.Symbol == symbol && record.Side == side && record.Status == "open" {
			if oldestOpenTrade == nil || record.OpenTime.Before(oldestOpenTrade.OpenTime) {
				oldestOpenTrade = record
			}
		}
	}

	return oldestOpenTrade, nil
}

// getAllTradeRecords 获取所有交易记录
func (tl *TradeLogger) getAllTradeRecords() ([]*TradeRecord, error) {
	files, err := ioutil.ReadDir(tl.tradeDir)
	if err != nil {
		return nil, fmt.Errorf("读取交易记录目录失败: %w", err)
	}

	var records []*TradeRecord
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// 只读取JSON文件
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		filepath := filepath.Join(tl.tradeDir, file.Name())
		data, err := ioutil.ReadFile(filepath)
		if err != nil {
			continue
		}

		var record TradeRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		records = append(records, &record)
	}

	return records, nil
}

// GetTradeRecords 获取交易记录（支持过滤）
func (tl *TradeLogger) GetTradeRecords(status string, limit int) ([]*TradeRecord, error) {
	tl.mutex.RLock()
	defer tl.mutex.RUnlock()

	records, err := tl.getAllTradeRecords()
	if err != nil {
		return nil, err
	}

	// 过滤状态
	var filtered []*TradeRecord
	for _, record := range records {
		if status == "" || record.Status == status {
			filtered = append(filtered, record)
		}
	}

	// 按时间倒序排序（最新的在前）
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}

	// 限制数量
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}

	return filtered, nil
}

// saveTradeRecord 保存交易记录到文件
func (tl *TradeLogger) saveTradeRecord(record *TradeRecord) error {
	// 文件名：trade_{trade_id}.json
	filename := fmt.Sprintf("trade_%s.json", record.TradeID)
	filepath := filepath.Join(tl.tradeDir, filename)

	// 序列化为JSON（带缩进，方便阅读）
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化交易记录失败: %w", err)
	}

	// 写入文件
	if err := ioutil.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("写入交易记录失败: %w", err)
	}

	return nil
}

// GetOpenTrades 获取所有未平仓的交易记录
func (tl *TradeLogger) GetOpenTrades() ([]*TradeRecord, error) {
	return tl.GetTradeRecords("open", 0)
}

// GetClosedTrades 获取所有已平仓的交易记录
func (tl *TradeLogger) GetClosedTrades(limit int) ([]*TradeRecord, error) {
	return tl.GetTradeRecords("closed", limit)
}

