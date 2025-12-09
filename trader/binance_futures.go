package trader

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2/futures"
)

// FuturesTrader 币安合约交易器
type FuturesTrader struct {
	client    *futures.Client
	apiKey    string // 保存API key用于签名
	secretKey string // 保存Secret key用于签名

	// 余额缓存
	cachedBalance     map[string]interface{}
	balanceCacheTime  time.Time
	balanceCacheMutex sync.RWMutex

	// 持仓缓存
	cachedPositions     []map[string]interface{}
	positionsCacheTime  time.Time
	positionsCacheMutex sync.RWMutex

	// 缓存有效期（15秒）
	cacheDuration time.Duration

	// 时间偏移（毫秒），用于补偿Docker环境时间偏差
	timeOffset int64
}

// NewFuturesTrader 创建合约交易器
func NewFuturesTrader(apiKey, secretKey string) *FuturesTrader {
	client := futures.NewClient(apiKey, secretKey)
	
	// 检测Binance服务器时间偏移，用于日志记录和问题诊断
	// 注意：go-binance库不支持SetTimeOffset或RecvWindow设置
	// 如果遇到时间偏差错误（-1021），需要：
	// 1. 确保Docker容器时间同步（docker-compose.yml已配置）
	// 2. 确保宿主机时间准确（使用NTP同步）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	var timeOffset int64 = 0
	serverTime, err := client.NewServerTimeService().Do(ctx)
	if err == nil {
		localTime := time.Now().Unix() * 1000
		timeOffset = serverTime - localTime
		log.Printf("✓ Binance时间偏移检测: %d ms (服务器时间: %d, 本地时间: %d)", 
			timeOffset, serverTime, localTime)
		if timeOffset > 5000 || timeOffset < -5000 {
			log.Printf("⚠ 警告：时间偏差较大（%d ms），可能触发-1021错误。建议同步系统时间。", timeOffset)
		}
	} else {
		log.Printf("⚠ 获取Binance服务器时间失败: %v", err)
	}
	
	return &FuturesTrader{
		client:        client,
		apiKey:        apiKey,    // 保存API key
		secretKey:     secretKey, // 保存Secret key用于签名
		cacheDuration: 15 * time.Second, // 15秒缓存
		timeOffset:    timeOffset,
	}
}

// GetBalance 获取账户余额（带缓存）
func (t *FuturesTrader) GetBalance() (map[string]interface{}, error) {
	// 先检查缓存是否有效
	t.balanceCacheMutex.RLock()
	if t.cachedBalance != nil && time.Since(t.balanceCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.balanceCacheTime)
		t.balanceCacheMutex.RUnlock()
		log.Printf("✓ 使用缓存的账户余额（缓存时间: %.1f秒前）", cacheAge.Seconds())
		return t.cachedBalance, nil
	}
	t.balanceCacheMutex.RUnlock()

	// 缓存过期或不存在，调用API
	log.Printf("🔄 缓存过期，正在调用币安API获取账户余额...")
	account, err := t.client.NewGetAccountService().Do(context.Background())
	if err != nil {
		log.Printf("❌ 币安API调用失败: %v", err)
		return nil, fmt.Errorf("获取账户信息失败: %w", err)
	}

	result := make(map[string]interface{})
	result["totalWalletBalance"], _ = strconv.ParseFloat(account.TotalWalletBalance, 64)
	result["availableBalance"], _ = strconv.ParseFloat(account.AvailableBalance, 64)
	result["totalUnrealizedProfit"], _ = strconv.ParseFloat(account.TotalUnrealizedProfit, 64)

	log.Printf("✓ 币安API返回: 总余额=%s, 可用=%s, 未实现盈亏=%s",
		account.TotalWalletBalance,
		account.AvailableBalance,
		account.TotalUnrealizedProfit)

	// 更新缓存
	t.balanceCacheMutex.Lock()
	t.cachedBalance = result
	t.balanceCacheTime = time.Now()
	t.balanceCacheMutex.Unlock()

	return result, nil
}

// GetPositions 获取所有持仓（带缓存）
func (t *FuturesTrader) GetPositions() ([]map[string]interface{}, error) {
	// 先检查缓存是否有效
	t.positionsCacheMutex.RLock()
	if t.cachedPositions != nil && time.Since(t.positionsCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.positionsCacheTime)
		t.positionsCacheMutex.RUnlock()
		log.Printf("✓ 使用缓存的持仓信息（缓存时间: %.1f秒前）", cacheAge.Seconds())
		return t.cachedPositions, nil
	}
	t.positionsCacheMutex.RUnlock()

	// 缓存过期或不存在，调用API
	log.Printf("🔄 缓存过期，正在调用币安API获取持仓信息...")
	positions, err := t.client.NewGetPositionRiskService().Do(context.Background())
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	var result []map[string]interface{}
	for _, pos := range positions {
		posAmt, _ := strconv.ParseFloat(pos.PositionAmt, 64)
		if posAmt == 0 {
			continue // 跳过无持仓的
		}

		posMap := make(map[string]interface{})
		posMap["symbol"] = pos.Symbol
		posMap["positionAmt"], _ = strconv.ParseFloat(pos.PositionAmt, 64)
		posMap["entryPrice"], _ = strconv.ParseFloat(pos.EntryPrice, 64)
		posMap["markPrice"], _ = strconv.ParseFloat(pos.MarkPrice, 64)
		posMap["unRealizedProfit"], _ = strconv.ParseFloat(pos.UnRealizedProfit, 64)
		posMap["leverage"], _ = strconv.ParseFloat(pos.Leverage, 64)
		posMap["liquidationPrice"], _ = strconv.ParseFloat(pos.LiquidationPrice, 64)

		// 判断方向
		if posAmt > 0 {
			posMap["side"] = "long"
		} else {
			posMap["side"] = "short"
		}

		result = append(result, posMap)
	}

	// 更新缓存
	t.positionsCacheMutex.Lock()
	t.cachedPositions = result
	t.positionsCacheTime = time.Now()
	t.positionsCacheMutex.Unlock()

	return result, nil
}

// SetMarginMode 设置仓位模式
func (t *FuturesTrader) SetMarginMode(symbol string, isCrossMargin bool) error {
	var marginType futures.MarginType
	if isCrossMargin {
		marginType = futures.MarginTypeCrossed
	} else {
		marginType = futures.MarginTypeIsolated
	}

	// 尝试设置仓位模式
	err := t.client.NewChangeMarginTypeService().
		Symbol(symbol).
		MarginType(marginType).
		Do(context.Background())

	marginModeStr := "全仓"
	if !isCrossMargin {
		marginModeStr = "逐仓"
	}

	if err != nil {
		// 如果错误信息包含"No need to change"，说明仓位模式已经是目标值
		if contains(err.Error(), "No need to change margin type") {
			log.Printf("  ✓ %s 仓位模式已是 %s", symbol, marginModeStr)
			return nil
		}
		// 如果有持仓，无法更改仓位模式，但不影响交易
		if contains(err.Error(), "Margin type cannot be changed if there exists position") {
			log.Printf("  ⚠️ %s 有持仓，无法更改仓位模式，继续使用当前模式", symbol)
			return nil
		}
		log.Printf("  ⚠️ 设置仓位模式失败: %v", err)
		// 不返回错误，让交易继续
		return nil
	}

	log.Printf("  ✓ %s 仓位模式已设置为 %s", symbol, marginModeStr)
	return nil
}

// SetLeverage 设置杠杆（智能判断+冷却期+时间戳错误重试）
func (t *FuturesTrader) SetLeverage(symbol string, leverage int) error {
	// 先尝试获取当前杠杆（从持仓信息）
	currentLeverage := 0
	positions, err := t.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] == symbol {
				if lev, ok := pos["leverage"].(float64); ok {
					currentLeverage = int(lev)
					break
				}
			}
		}
	}

	// 如果当前杠杆已经是目标杠杆，跳过
	if currentLeverage == leverage && currentLeverage > 0 {
		log.Printf("  ✓ %s 杠杆已是 %dx，无需切换", symbol, leverage)
		return nil
	}

	// 重试机制：最多重试3次，处理时间戳错误（-1021）
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// 切换杠杆
		_, err = t.client.NewChangeLeverageService().
			Symbol(symbol).
			Leverage(leverage).
			Do(context.Background())

		if err == nil {
			// 成功
			log.Printf("  ✓ %s 杠杆已切换为 %dx", symbol, leverage)
			// 切换杠杆后等待5秒（避免冷却期错误）
			log.Printf("  ⏱ 等待5秒冷却期...")
			time.Sleep(5 * time.Second)
			return nil
		}

		// 检查错误类型
		errStr := err.Error()
		
		// 如果错误信息包含"No need to change"，说明杠杆已经是目标值
		if contains(errStr, "No need to change") {
			log.Printf("  ✓ %s 杠杆已是 %dx", symbol, leverage)
			return nil
		}

		// 如果是时间戳错误（-1021），尝试重试
		if contains(errStr, "-1021") || contains(errStr, "Timestamp") {
			if attempt < maxRetries {
				// 获取服务器时间，计算时间差
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				serverTime, serverErr := t.client.NewServerTimeService().Do(ctx)
				cancel()
				
				if serverErr == nil {
					localTime := time.Now().Unix() * 1000
					timeOffset := serverTime - localTime
					log.Printf("  ⚠ 时间戳错误（-1021），检测到时间偏移: %d ms", timeOffset)
					
					// 如果时间偏移较大，等待一段时间让时间同步
					if timeOffset > 1000 || timeOffset < -1000 {
						waitTime := time.Duration(abs(timeOffset)) * time.Millisecond
						if waitTime > 2*time.Second {
							waitTime = 2 * time.Second // 最多等待2秒
						}
						log.Printf("  ⏱ 等待 %v 后重试（第 %d/%d 次）...", waitTime, attempt+1, maxRetries)
						time.Sleep(waitTime)
					} else {
						// 时间偏移较小，短暂等待后重试
						log.Printf("  ⏱ 短暂等待后重试（第 %d/%d 次）...", attempt+1, maxRetries)
						time.Sleep(500 * time.Millisecond)
					}
					continue // 重试
				} else {
					// 无法获取服务器时间，短暂等待后重试
					log.Printf("  ⚠ 无法获取服务器时间，等待后重试（第 %d/%d 次）...", attempt+1, maxRetries)
					time.Sleep(1 * time.Second)
					continue // 重试
				}
			} else {
				// 最后一次重试也失败
				return fmt.Errorf("设置杠杆失败（时间戳错误，已重试%d次）: %w", maxRetries, err)
			}
		}

		// 其他错误，直接返回
		return fmt.Errorf("设置杠杆失败: %w", err)
	}

	// 理论上不会到达这里
	return fmt.Errorf("设置杠杆失败: %w", err)
}

// abs 返回绝对值
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// OpenLong 开多仓
func (t *FuturesTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 注意：仓位模式应该由调用方（AutoTrader）在开仓前通过 SetMarginMode 设置

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价买入订单
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("开多仓失败: %w", err)
	}

	log.Printf("✓ 开多仓成功: %s 数量: %s", symbol, quantityStr)
	log.Printf("  订单ID: %d", order.OrderID)

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// OpenShort 开空仓
func (t *FuturesTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 注意：仓位模式应该由调用方（AutoTrader）在开仓前通过 SetMarginMode 设置

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价卖出订单
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeShort).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("开空仓失败: %w", err)
	}

	log.Printf("✓ 开空仓成功: %s 数量: %s", symbol, quantityStr)
	log.Printf("  订单ID: %d", order.OrderID)

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CloseLong 平多仓
func (t *FuturesTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "long" {
				quantity = pos["positionAmt"].(float64)
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("没有找到 %s 的多仓", symbol)
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价卖出订单（平多）
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeSell).
		PositionSide(futures.PositionSideTypeLong).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("平多仓失败: %w", err)
	}

	log.Printf("✓ 平多仓成功: %s 数量: %s", symbol, quantityStr)

	// 平仓后取消该币种的所有挂单（止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消挂单失败: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CloseShort 平空仓
func (t *FuturesTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "short" {
				quantity = -pos["positionAmt"].(float64) // 空仓数量是负的，取绝对值
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("没有找到 %s 的空仓", symbol)
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 创建市价买入订单（平空）
	order, err := t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(futures.SideTypeBuy).
		PositionSide(futures.PositionSideTypeShort).
		Type(futures.OrderTypeMarket).
		Quantity(quantityStr).
		Do(context.Background())

	if err != nil {
		return nil, fmt.Errorf("平空仓失败: %w", err)
	}

	log.Printf("✓ 平空仓成功: %s 数量: %s", symbol, quantityStr)

	// 平仓后取消该币种的所有挂单（止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消挂单失败: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = order.OrderID
	result["symbol"] = order.Symbol
	result["status"] = order.Status
	return result, nil
}

// CancelAllOrders 取消该币种的所有挂单
func (t *FuturesTrader) CancelAllOrders(symbol string) error {
	err := t.client.NewCancelAllOpenOrdersService().
		Symbol(symbol).
		Do(context.Background())

	if err != nil {
		return fmt.Errorf("取消挂单失败: %w", err)
	}

	log.Printf("  ✓ 已取消 %s 的所有挂单", symbol)
	return nil
}

// GetMarketPrice 获取市场价格
func (t *FuturesTrader) GetMarketPrice(symbol string) (float64, error) {
	prices, err := t.client.NewListPricesService().Symbol(symbol).Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("获取价格失败: %w", err)
	}

	if len(prices) == 0 {
		return 0, fmt.Errorf("未找到价格")
	}

	price, err := strconv.ParseFloat(prices[0].Price, 64)
	if err != nil {
		return 0, err
	}

	return price, nil
}

// CalculatePositionSize 计算仓位大小
func (t *FuturesTrader) CalculatePositionSize(balance, riskPercent, price float64, leverage int) float64 {
	riskAmount := balance * (riskPercent / 100.0)
	positionValue := riskAmount * float64(leverage)
	quantity := positionValue / price
	return quantity
}

// SetStopLoss 设置止损单
func (t *FuturesTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	var side futures.SideType
	var posSide futures.PositionSideType

	if positionSide == "LONG" {
		side = futures.SideTypeSell
		posSide = futures.PositionSideTypeLong
	} else {
		side = futures.SideTypeBuy
		posSide = futures.PositionSideTypeShort
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return err
	}

	_, err = t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(posSide).
		Type(futures.OrderTypeStopMarket).
		StopPrice(fmt.Sprintf("%.8f", stopPrice)).
		Quantity(quantityStr).
		WorkingType(futures.WorkingTypeContractPrice).
		ClosePosition(true).
		Do(context.Background())

	if err != nil {
		return fmt.Errorf("设置止损失败: %w", err)
	}

	log.Printf("  止损价设置: %.4f", stopPrice)
	return nil
}

// SetTakeProfit 设置止盈单
func (t *FuturesTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	var side futures.SideType
	var posSide futures.PositionSideType

	if positionSide == "LONG" {
		side = futures.SideTypeSell
		posSide = futures.PositionSideTypeLong
	} else {
		side = futures.SideTypeBuy
		posSide = futures.PositionSideTypeShort
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return err
	}

	_, err = t.client.NewCreateOrderService().
		Symbol(symbol).
		Side(side).
		PositionSide(posSide).
		Type(futures.OrderTypeTakeProfitMarket).
		StopPrice(fmt.Sprintf("%.8f", takeProfitPrice)).
		Quantity(quantityStr).
		WorkingType(futures.WorkingTypeContractPrice).
		ClosePosition(true).
		Do(context.Background())

	if err != nil {
		return fmt.Errorf("设置止盈失败: %w", err)
	}

	log.Printf("  止盈价设置: %.4f", takeProfitPrice)
	return nil
}

// GetSymbolPrecision 获取交易对的数量精度
func (t *FuturesTrader) GetSymbolPrecision(symbol string) (int, error) {
	exchangeInfo, err := t.client.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		return 0, fmt.Errorf("获取交易规则失败: %w", err)
	}

	for _, s := range exchangeInfo.Symbols {
		if s.Symbol == symbol {
			// 从LOT_SIZE filter获取精度
			for _, filter := range s.Filters {
				if filter["filterType"] == "LOT_SIZE" {
					stepSize := filter["stepSize"].(string)
					precision := calculatePrecision(stepSize)
					log.Printf("  %s 数量精度: %d (stepSize: %s)", symbol, precision, stepSize)
					return precision, nil
				}
			}
		}
	}

	log.Printf("  ⚠ %s 未找到精度信息，使用默认精度3", symbol)
	return 3, nil // 默认精度为3
}

// calculatePrecision 从stepSize计算精度
func calculatePrecision(stepSize string) int {
	// 去除尾部的0
	stepSize = trimTrailingZeros(stepSize)

	// 查找小数点
	dotIndex := -1
	for i := 0; i < len(stepSize); i++ {
		if stepSize[i] == '.' {
			dotIndex = i
			break
		}
	}

	// 如果没有小数点或小数点在最后，精度为0
	if dotIndex == -1 || dotIndex == len(stepSize)-1 {
		return 0
	}

	// 返回小数点后的位数
	return len(stepSize) - dotIndex - 1
}

// trimTrailingZeros 去除尾部的0
func trimTrailingZeros(s string) string {
	// 如果没有小数点，直接返回
	if !stringContains(s, ".") {
		return s
	}

	// 从后向前遍历，去除尾部的0
	for len(s) > 0 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}

	// 如果最后一位是小数点，也去掉
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}

	return s
}

// FormatQuantity 格式化数量到正确的精度
func (t *FuturesTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	precision, err := t.GetSymbolPrecision(symbol)
	if err != nil {
		// 如果获取失败，使用默认格式
		return fmt.Sprintf("%.3f", quantity), nil
	}

	format := fmt.Sprintf("%%.%df", precision)
	return fmt.Sprintf(format, quantity), nil
}

// signRequest 对请求进行HMAC SHA256签名
func (t *FuturesTrader) signRequest(query string) string {
	mac := hmac.New(sha256.New, []byte(t.secretKey))
	mac.Write([]byte(query))
	return hex.EncodeToString(mac.Sum(nil))
}

// GetUserTrades 获取用户交易历史（从币安API直接获取真实交易记录）
// symbol: 交易对，如 "BTCUSDT"，空字符串表示所有交易对（注意：币安API要求symbol必须指定）
// limit: 返回的交易记录数量（最大1000）
// startTime: 开始时间（可选）
// endTime: 结束时间（可选）
func (t *FuturesTrader) GetUserTrades(symbol string, limit int, startTime, endTime *time.Time) ([]map[string]interface{}, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000 // 币安API限制：最多1000条
	}
	
	if symbol == "" {
		return nil, fmt.Errorf("币安API要求必须指定交易对（symbol）")
	}

	// 使用HTTP请求直接调用币安API（因为go-binance库可能没有这个方法）
	baseURL := "https://fapi.binance.com"
	path := "/fapi/v1/userTrades"
	
	// 构建查询参数
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("limit", strconv.Itoa(limit))
	if startTime != nil {
		params.Set("startTime", strconv.FormatInt(startTime.UnixMilli(), 10))
	}
	if endTime != nil {
		params.Set("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli()+t.timeOffset, 10))
	
	// 签名请求
	queryString := params.Encode()
	signature := t.signRequest(queryString)
	params.Set("signature", signature)
	
	// 构建完整URL
	fullURL := fmt.Sprintf("%s%s?%s", baseURL, path, params.Encode())
	
	// 创建HTTP请求
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %w", err)
	}
	
	// 设置请求头
	req.Header.Set("X-MBX-APIKEY", t.apiKey)
	
	// 发送请求
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()
	
	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	
	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("币安API返回错误: %s (状态码: %d)", string(body), resp.StatusCode)
	}
	
	// 解析JSON响应
	var trades []struct {
		Symbol          string `json:"symbol"`
		ID              int64  `json:"id"`
		OrderID         int64  `json:"orderId"`
		Side            string `json:"side"`
		PositionSide    string `json:"positionSide"`
		Price           string `json:"price"`
		Qty             string `json:"qty"`
		QuoteQty        string `json:"quoteQty"`
		Commission      string `json:"commission"`
		CommissionAsset string `json:"commissionAsset"`
		RealizedPnl     string `json:"realizedPnl"`
		Time            int64  `json:"time"`
	}
	
	if err := json.Unmarshal(body, &trades); err != nil {
		return nil, fmt.Errorf("解析JSON响应失败: %w", err)
	}
	
	// 转换为map格式
	result := make([]map[string]interface{}, len(trades))
	for i, trade := range trades {
		price, _ := strconv.ParseFloat(trade.Price, 64)
		qty, _ := strconv.ParseFloat(trade.Qty, 64)
		quoteQty, _ := strconv.ParseFloat(trade.QuoteQty, 64)
		commission, _ := strconv.ParseFloat(trade.Commission, 64)
		realizedPnl, _ := strconv.ParseFloat(trade.RealizedPnl, 64)

		result[i] = map[string]interface{}{
			"symbol":          trade.Symbol,
			"id":              trade.ID,
			"order_id":        trade.OrderID,
			"side":            trade.Side,           // "BUY" or "SELL"
			"position_side":   trade.PositionSide,   // "LONG" or "SHORT"
			"price":           price,
			"qty":             qty,
			"quote_qty":       quoteQty,
			"commission":      commission,
			"commission_asset": trade.CommissionAsset,
			"realized_pnl":    realizedPnl,
			"time":            trade.Time,
			"time_in_ms":      trade.Time,
		}
	}

	log.Printf("✓ 从币安API获取到 %d 条真实交易记录（symbol: %s）", len(result), symbol)
	return result, nil
}

// 辅助函数
func contains(s, substr string) bool {
	return len(s) >= len(substr) && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
