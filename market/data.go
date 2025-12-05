package market

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// Get 获取指定代币的市场数据
func Get(symbol string) (*Data, error) {
	var klines3m, klines1h, klines4h []Kline
	var err error
	// 标准化symbol
	symbol = Normalize(symbol)
	// 获取3分钟K线数据 (最近10个)
	klines3m, err = WSMonitorCli.GetCurrentKlines(symbol, "3m") // 多获取一些用于计算
	if err != nil {
		return nil, fmt.Errorf("获取3分钟K线失败: %v", err)
	}

	// 获取1小时K线数据 (最近10个)
	klines1h, err = WSMonitorCli.GetCurrentKlines(symbol, "1h") // 多获取用于计算指标
	if err != nil {
		return nil, fmt.Errorf("获取1小时K线失败: %v", err)
	}

	// 获取4小时K线数据 (最近10个)
	klines4h, err = WSMonitorCli.GetCurrentKlines(symbol, "4h") // 多获取用于计算指标
	if err != nil {
		return nil, fmt.Errorf("获取4小时K线失败: %v", err)
	}

	// 计算当前指标 (基于3分钟最新数据)
	currentPrice := klines3m[len(klines3m)-1].Close
	currentEMA20 := calculateEMA(klines3m, 20)
	currentMACD := calculateMACD(klines3m)
	currentRSI7 := calculateRSI(klines3m, 7)

	// 计算价格变化百分比
	// 1小时价格变化 = 20个3分钟K线前的价格
	priceChange1h := 0.0
	if len(klines3m) >= 21 { // 至少需要21根K线 (当前 + 20根前)
		price1hAgo := klines3m[len(klines3m)-21].Close
		if price1hAgo > 0 {
			priceChange1h = ((currentPrice - price1hAgo) / price1hAgo) * 100
		}
	}

	// 4小时价格变化 = 1个4小时K线前的价格
	priceChange4h := 0.0
	if len(klines4h) >= 2 {
		price4hAgo := klines4h[len(klines4h)-2].Close
		if price4hAgo > 0 {
			priceChange4h = ((currentPrice - price4hAgo) / price4hAgo) * 100
		}
	}

	// 获取OI数据
	oiData, err := getOpenInterestData(symbol)
	if err != nil {
		// OI失败不影响整体,使用默认值
		oiData = &OIData{Latest: 0, Average: 0}
	}

	// 获取Funding Rate和标记价格、指数价格
	fundingRate, markPrice, indexPrice := getFundingRateAndPrices(symbol)

	// 获取重要API数据（失败不影响主流程）
	apiClient := NewAPIClient()
	
	// 获取多空持仓比（1小时周期）
	longShortRatio, _ := apiClient.GetLongShortRatio(symbol, "1h")
	
	// 获取大户多空持仓比（1小时周期）
	topTraderRatio, _ := apiClient.GetTopTraderLongShortRatio(symbol, "1h")
	
	// 获取24小时统计
	ticker24hr, _ := apiClient.Get24hrTicker(symbol)

	// 计算日内系列数据
	intradayData := calculateIntradaySeries(klines3m)

	// 计算1小时数据
	hourlyData := calculateHourlyData(klines1h)

	// 计算长期数据
	longerTermData := calculateLongerTermData(klines4h)

	return &Data{
		Symbol:            symbol,
		CurrentPrice:      currentPrice,
		PriceChange1h:     priceChange1h,
		PriceChange4h:     priceChange4h,
		CurrentEMA20:      currentEMA20,
		CurrentMACD:       currentMACD,
		CurrentRSI7:       currentRSI7,
		OpenInterest:      oiData,
		FundingRate:       fundingRate,
		IntradaySeries:    intradayData,
		HourlyContext:     hourlyData,
		LongerTermContext: longerTermData,
		// 新增的重要API数据
		LongShortRatio: longShortRatio,
		TopTraderRatio: topTraderRatio,
		Ticker24hr:      ticker24hr,
		MarkPrice:       markPrice,
		IndexPrice:      indexPrice,
	}, nil
}

// calculateEMA 计算EMA
func calculateEMA(klines []Kline, period int) float64 {
	if len(klines) < period {
		return 0
	}

	// 计算SMA作为初始EMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += klines[i].Close
	}
	ema := sum / float64(period)

	// 计算EMA
	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(klines); i++ {
		ema = (klines[i].Close-ema)*multiplier + ema
	}

	return ema
}

// calculateMACD 计算MACD
func calculateMACD(klines []Kline) float64 {
	if len(klines) < 26 {
		return 0
	}

	// 计算12期和26期EMA
	ema12 := calculateEMA(klines, 12)
	ema26 := calculateEMA(klines, 26)

	// MACD = EMA12 - EMA26
	return ema12 - ema26
}

// calculateRSI 计算RSI
func calculateRSI(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	gains := 0.0
	losses := 0.0

	// 计算初始平均涨跌幅
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gains += change
		} else {
			losses += -change
		}
	}

	avgGain := gains / float64(period)
	avgLoss := losses / float64(period)

	// 使用Wilder平滑方法计算后续RSI
	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			avgGain = (avgGain*float64(period-1) + change) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) + (-change)) / float64(period)
		}
	}

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	return rsi
}

// calculateATR 计算ATR
func calculateATR(klines []Kline, period int) float64 {
	if len(klines) <= period {
		return 0
	}

	trs := make([]float64, len(klines))
	for i := 1; i < len(klines); i++ {
		high := klines[i].High
		low := klines[i].Low
		prevClose := klines[i-1].Close

		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)

		trs[i] = math.Max(tr1, math.Max(tr2, tr3))
	}

	// 计算初始ATR
	sum := 0.0
	for i := 1; i <= period; i++ {
		sum += trs[i]
	}
	atr := sum / float64(period)

	// Wilder平滑
	for i := period + 1; i < len(klines); i++ {
		atr = (atr*float64(period-1) + trs[i]) / float64(period)
	}

	return atr
}

// calculateIntradaySeries 计算日内系列数据
func calculateIntradaySeries(klines []Kline) *IntradayData {
	data := &IntradayData{
		MidPrices:   make([]float64, 0, 10),
		EMA20Values: make([]float64, 0, 10),
		MACDValues:  make([]float64, 0, 10),
		RSI7Values:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 获取最近10个数据点
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		data.MidPrices = append(data.MidPrices, klines[i].Close)

		// 计算每个点的EMA20
		if i >= 19 {
			ema20 := calculateEMA(klines[:i+1], 20)
			data.EMA20Values = append(data.EMA20Values, ema20)
		}

		// 计算每个点的MACD
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}

		// 计算每个点的RSI
		if i >= 7 {
			rsi7 := calculateRSI(klines[:i+1], 7)
			data.RSI7Values = append(data.RSI7Values, rsi7)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// calculateHourlyData 计算1小时数据
func calculateHourlyData(klines []Kline) *HourlyData {
	data := &HourlyData{
		MACDValues:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 计算EMA
	data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)

	// 计算ATR
	data.ATR3 = calculateATR(klines, 3)
	data.ATR14 = calculateATR(klines, 14)

	// 计算成交量
	if len(klines) > 0 {
		data.CurrentVolume = klines[len(klines)-1].Volume
		// 计算平均成交量
		sum := 0.0
		for _, k := range klines {
			sum += k.Volume
		}
		data.AverageVolume = sum / float64(len(klines))
	}

	// 计算MACD和RSI序列
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// calculateLongerTermData 计算长期数据
func calculateLongerTermData(klines []Kline) *LongerTermData {
	data := &LongerTermData{
		MACDValues:  make([]float64, 0, 10),
		RSI14Values: make([]float64, 0, 10),
	}

	// 计算EMA
	data.EMA20 = calculateEMA(klines, 20)
	data.EMA50 = calculateEMA(klines, 50)

	// 计算ATR
	data.ATR3 = calculateATR(klines, 3)
	data.ATR14 = calculateATR(klines, 14)

	// 计算成交量
	if len(klines) > 0 {
		data.CurrentVolume = klines[len(klines)-1].Volume
		// 计算平均成交量
		sum := 0.0
		for _, k := range klines {
			sum += k.Volume
		}
		data.AverageVolume = sum / float64(len(klines))
	}

	// 计算MACD和RSI序列
	start := len(klines) - 10
	if start < 0 {
		start = 0
	}

	for i := start; i < len(klines); i++ {
		if i >= 25 {
			macd := calculateMACD(klines[:i+1])
			data.MACDValues = append(data.MACDValues, macd)
		}
		if i >= 14 {
			rsi14 := calculateRSI(klines[:i+1], 14)
			data.RSI14Values = append(data.RSI14Values, rsi14)
		}
	}

	return data
}

// getOpenInterestData 获取OI数据
func getOpenInterestData(symbol string) (*OIData, error) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/openInterest?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		OpenInterest string `json:"openInterest"`
		Symbol       string `json:"symbol"`
		Time         int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	oi, _ := strconv.ParseFloat(result.OpenInterest, 64)

	return &OIData{
		Latest:  oi,
		Average: oi * 0.999, // 近似平均值
	}, nil
}

// getFundingRateAndPrices 获取资金费率、标记价格和指数价格
func getFundingRateAndPrices(symbol string) (float64, float64, float64) {
	url := fmt.Sprintf("https://fapi.binance.com/fapi/v1/premiumIndex?symbol=%s", symbol)

	resp, err := http.Get(url)
	if err != nil {
		return 0, 0, 0
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, 0
	}

	var result struct {
		Symbol          string `json:"symbol"`
		MarkPrice       string `json:"markPrice"`
		IndexPrice      string `json:"indexPrice"`
		LastFundingRate string `json:"lastFundingRate"`
		NextFundingTime int64  `json:"nextFundingTime"`
		InterestRate    string `json:"interestRate"`
		Time            int64  `json:"time"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return 0, 0, 0
	}

	rate, _ := strconv.ParseFloat(result.LastFundingRate, 64)
	markPrice, _ := strconv.ParseFloat(result.MarkPrice, 64)
	indexPrice, _ := strconv.ParseFloat(result.IndexPrice, 64)
	
	return rate, markPrice, indexPrice
}

// getLongShortRatioInterpretation 获取多空比解释
func getLongShortRatioInterpretation(ratio float64) string {
	if ratio > 1.2 {
		return "多头占优（>1.2）"
	} else if ratio > 1.0 {
		return "多头略优（1.0-1.2）"
	} else if ratio > 0.8 {
		return "空头略优（0.8-1.0）"
	} else {
		return "空头占优（<0.8）"
	}
}

// Format 格式化输出市场数据
func Format(data *Data) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Price=%.2f EMA20=%.3f MACD=%.3f RSI7=%.3f",
		data.CurrentPrice, data.CurrentEMA20, data.CurrentMACD, data.CurrentRSI7))

	if data.OpenInterest != nil {
		sb.WriteString(fmt.Sprintf(" OI=%.2f AvgOI=%.2f", data.OpenInterest.Latest, data.OpenInterest.Average))
	}

	sb.WriteString(fmt.Sprintf(" Funding=%.2e\n\n", data.FundingRate))

	if data.IntradaySeries != nil {
		sb.WriteString("Intraday series (3m, latest 5):\n")

		if len(data.IntradaySeries.MidPrices) > 0 {
			sb.WriteString(fmt.Sprintf("Prices: %s\n", formatFloatSlice(data.IntradaySeries.MidPrices, 5)))
		}

		if len(data.IntradaySeries.EMA20Values) > 0 {
			sb.WriteString(fmt.Sprintf("EMA20: %s\n", formatFloatSlice(data.IntradaySeries.EMA20Values, 5)))
		}

		if len(data.IntradaySeries.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf("MACD: %s\n", formatFloatSlice(data.IntradaySeries.MACDValues, 5)))
		}

		if len(data.IntradaySeries.RSI7Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI7: %s\n", formatFloatSlice(data.IntradaySeries.RSI7Values, 5)))
		}

		if len(data.IntradaySeries.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf("RSI14: %s\n", formatFloatSlice(data.IntradaySeries.RSI14Values, 5)))
		}
		sb.WriteString("\n")
	}

	if data.HourlyContext != nil {
		sb.WriteString("Hourly (1h): ")
		sb.WriteString(fmt.Sprintf("EMA20=%.3f EMA50=%.3f ATR3=%.3f ATR14=%.3f Vol=%.3f AvgVol=%.3f",
			data.HourlyContext.EMA20, data.HourlyContext.EMA50,
			data.HourlyContext.ATR3, data.HourlyContext.ATR14,
			data.HourlyContext.CurrentVolume, data.HourlyContext.AverageVolume))
		if len(data.HourlyContext.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf(" MACD=%s", formatFloatSlice(data.HourlyContext.MACDValues, 3)))
		}
		if len(data.HourlyContext.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf(" RSI14=%s", formatFloatSlice(data.HourlyContext.RSI14Values, 3)))
		}
		sb.WriteString("\n\n")
	}

	if data.LongerTermContext != nil {
		sb.WriteString("4h: ")
		sb.WriteString(fmt.Sprintf("EMA20=%.3f EMA50=%.3f ATR3=%.3f ATR14=%.3f Vol=%.3f AvgVol=%.3f",
			data.LongerTermContext.EMA20, data.LongerTermContext.EMA50,
			data.LongerTermContext.ATR3, data.LongerTermContext.ATR14,
			data.LongerTermContext.CurrentVolume, data.LongerTermContext.AverageVolume))
		if len(data.LongerTermContext.MACDValues) > 0 {
			sb.WriteString(fmt.Sprintf(" MACD=%s", formatFloatSlice(data.LongerTermContext.MACDValues, 3)))
		}
		if len(data.LongerTermContext.RSI14Values) > 0 {
			sb.WriteString(fmt.Sprintf(" RSI14=%s", formatFloatSlice(data.LongerTermContext.RSI14Values, 3)))
		}
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// formatFloatSlice 格式化float64切片为字符串
// maxPoints: 最多输出的数据点数量（用于减少token使用）
func formatFloatSlice(values []float64, maxPoints int) string {
	if maxPoints <= 0 {
		maxPoints = 5 // 默认最多5个点
	}
	
	// 只取最后maxPoints个数据点（最新的）
	start := len(values) - maxPoints
	if start < 0 {
		start = 0
	}
	
	valuesToFormat := values[start:]
	strValues := make([]string, len(valuesToFormat))
	for i, v := range valuesToFormat {
		strValues[i] = fmt.Sprintf("%.3f", v)
	}
	return "[" + strings.Join(strValues, ", ") + "]"
}

// Normalize 标准化symbol,确保是USDT交易对
func Normalize(symbol string) string {
	symbol = strings.ToUpper(symbol)
	if strings.HasSuffix(symbol, "USDT") {
		return symbol
	}
	return symbol + "USDT"
}

// parseFloat 解析float值
func parseFloat(v interface{}) (float64, error) {
	switch val := v.(type) {
	case string:
		return strconv.ParseFloat(val, 64)
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("unsupported type: %T", v)
	}
}
