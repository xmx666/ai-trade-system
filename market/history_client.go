package market

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	// Binance API限制：单次请求最多1000条K线数据
	BinanceMaxKlinesPerRequest = 1000
	// krnos标准预测参数
	HistoryStepsForPrediction = 450  // 历史数据步数（标准450步）
	PredictionSteps = 120            // 预测步数（标准120步）
	// 预测所需的最小数据点数
	MinDataPointsForPrediction = 100
)

// HistoryClient 历史数据客户端，处理币安API限制
type HistoryClient struct {
	apiClient *APIClient
}

// NewHistoryClient 创建历史数据客户端
func NewHistoryClient() *HistoryClient {
	return &HistoryClient{
		apiClient: NewAPIClient(),
	}
}

// GetHistoricalKlines 获取历史K线数据（处理币安API限制）
// 如果需要的数据超过1000条，会自动分批获取并合并
//
// Args:
//   - symbol: 币种符号，如 "BTCUSDT"
//   - interval: K线间隔，如 "3m", "1h", "4h"
//   - limit: 需要获取的数据条数
//   - endTime: 结束时间（可选，nil表示获取最新数据）
//
// Returns:
//   - []Kline: K线数据数组（按时间从旧到新排序）
//   - error: 错误信息
func (hc *HistoryClient) GetHistoricalKlines(symbol, interval string, limit int, endTime *time.Time) ([]Kline, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than 0")
	}

	// 如果需要的数量在单次请求限制内，直接获取
	if limit <= BinanceMaxKlinesPerRequest {
		return hc.getKlinesWithEndTime(symbol, interval, limit, endTime)
	}

	// 需要分批获取
	log.Printf("📊 %s 需要获取 %d 条K线数据，将分批获取（币安API限制：%d条/次）", symbol, limit, BinanceMaxKlinesPerRequest)

	var allKlines []Kline
	remaining := limit
	currentEndTime := endTime

	// 分批获取，每次最多1000条
	for remaining > 0 {
		batchSize := BinanceMaxKlinesPerRequest
		if remaining < BinanceMaxKlinesPerRequest {
			batchSize = remaining
		}

		klines, err := hc.getKlinesWithEndTime(symbol, interval, batchSize, currentEndTime)
		if err != nil {
			return nil, fmt.Errorf("获取K线数据失败: %w", err)
		}

		if len(klines) == 0 {
			// 没有更多数据了
			break
		}

		// 将新获取的数据添加到结果中（注意：币安返回的数据是从旧到新，但我们需要合并时保持顺序）
		allKlines = append(klines, allKlines...)

		// 更新结束时间为最早的那条K线的开始时间
		if len(klines) > 0 {
			earliestTime := time.Unix(klines[0].OpenTime/1000, 0)
			currentEndTime = &earliestTime
		}

		remaining -= len(klines)

		// 如果获取的数据少于请求的数量，说明已经获取完所有可用数据
		if len(klines) < batchSize {
			break
		}

		// 避免请求过快，稍微延迟
		time.Sleep(100 * time.Millisecond)
	}

	// 只返回需要的数量（最新的limit条）
	if len(allKlines) > limit {
		allKlines = allKlines[len(allKlines)-limit:]
	}

	log.Printf("✓ %s 成功获取 %d 条K线数据（请求 %d 条）", symbol, len(allKlines), limit)

	return allKlines, nil
}

// getKlinesWithEndTime 获取指定结束时间的K线数据
func (hc *HistoryClient) getKlinesWithEndTime(symbol, interval string, limit int, endTime *time.Time) ([]Kline, error) {
	url := fmt.Sprintf("%s/fapi/v1/klines", baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("symbol", symbol)
	q.Add("interval", interval)
	q.Add("limit", strconv.Itoa(limit))
	
	// 如果指定了结束时间，添加endTime参数
	if endTime != nil {
		q.Add("endTime", strconv.FormatInt(endTime.UnixMilli(), 10))
	}
	
	req.URL.RawQuery = q.Encode()

	resp, err := hc.apiClient.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var klineResponses []KlineResponse
	err = json.Unmarshal(body, &klineResponses)
	if err != nil {
		return nil, err
	}

	var klines []Kline
	for _, kr := range klineResponses {
		kline, err := parseKline(kr)
		if err != nil {
			log.Printf("解析K线数据失败: %v", err)
			continue
		}
		klines = append(klines, kline)
	}

	return klines, nil
}

// GetLatestKlinesForPrediction 获取用于预测的最新K线数据
// 自动获取足够的数据点，确保数据是最新的
//
// Args:
//   - symbol: 币种符号
//   - interval: K线间隔（建议使用 "3m" 用于短期预测）
//   - count: 需要的数据点数（标准450步，最少100）
//
// Returns:
//   - []Kline: K线数据数组（按时间从旧到新排序，最新的在最后）
//   - error: 错误信息
func (hc *HistoryClient) GetLatestKlinesForPrediction(symbol, interval string, count int) ([]Kline, error) {
	// 确保至少获取100条数据
	if count < MinDataPointsForPrediction {
		count = MinDataPointsForPrediction
		log.Printf("⚠️  数据点数 %d 少于最小值 %d，已调整为 %d", count, MinDataPointsForPrediction, MinDataPointsForPrediction)
	}
	
	// 如果请求的数据超过标准450步，记录警告
	if count > HistoryStepsForPrediction {
		log.Printf("⚠️  请求数据点数 %d 超过标准 %d 步，可能影响预测准确性", count, HistoryStepsForPrediction)
	}

	// 获取最新的K线数据（不指定endTime，获取最新数据）
	klines, err := hc.GetHistoricalKlines(symbol, interval, count, nil)
	if err != nil {
		return nil, fmt.Errorf("获取最新K线数据失败: %w", err)
	}

	// 验证数据是否足够
	if len(klines) < MinDataPointsForPrediction {
		return nil, fmt.Errorf("获取的数据不足：只有 %d 条，至少需要 %d 条", len(klines), MinDataPointsForPrediction)
	}

	// 验证数据是否是最新的（最后一条K线的时间应该在最近几分钟内）
	latestKline := klines[len(klines)-1]
	latestTime := time.Unix(latestKline.OpenTime/1000, 0)
	timeSinceLatest := time.Since(latestTime)

	// 根据K线间隔判断数据是否足够新
	intervalDuration := getIntervalDuration(interval)
	maxStaleness := intervalDuration * 2 // 允许最多2个间隔的延迟

	if timeSinceLatest > maxStaleness {
		log.Printf("⚠️  %s 最新K线数据可能过时：最新数据时间 %s，距今 %v", 
			symbol, latestTime.Format("2006-01-02 15:04:05"), timeSinceLatest)
		// 不返回错误，但记录警告
	}

	log.Printf("✓ %s 获取到 %d 条最新K线数据（最新时间: %s）", 
		symbol, len(klines), latestTime.Format("2006-01-02 15:04:05"))

	return klines, nil
}

// getIntervalDuration 获取K线间隔的时长
func getIntervalDuration(interval string) time.Duration {
	switch interval {
	case "1m":
		return 1 * time.Minute
	case "3m":
		return 3 * time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	case "1h":
		return 1 * time.Hour
	case "2h":
		return 2 * time.Hour
	case "4h":
		return 4 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "8h":
		return 8 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return 3 * time.Minute // 默认3分钟
	}
}

