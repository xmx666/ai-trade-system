package market

import "time"

// Data 市场数据结构
type Data struct {
	Symbol            string
	CurrentPrice      float64
	PriceChange1h     float64 // 1小时价格变化百分比
	PriceChange4h     float64 // 4小时价格变化百分比
	CurrentEMA20      float64
	CurrentMACD       float64
	CurrentRSI7       float64
	OpenInterest      *OIData
	FundingRate       float64
	IntradaySeries    *IntradayData
	HourlyContext     *HourlyData // 1小时时间框架数据
	LongerTermContext *LongerTermData
	// 新增的重要API数据
	LongShortRatio      *LongShortRatioData // 多空持仓比
	TopTraderRatio      *LongShortRatioData // 大户多空持仓比
	Ticker24hr          *Ticker24hrData     // 24小时统计
	MarkPrice           float64              // 标记价格
	IndexPrice          float64              // 指数价格
}

// OIData Open Interest数据
type OIData struct {
	Latest  float64
	Average float64
}

// IntradayData 日内数据(3分钟间隔)
type IntradayData struct {
	MidPrices   []float64
	EMA20Values []float64
	MACDValues  []float64
	RSI7Values  []float64
	RSI14Values []float64
}

// HourlyData 1小时时间框架数据
type HourlyData struct {
	EMA20         float64
	EMA50         float64
	ATR3          float64
	ATR14         float64
	CurrentVolume float64
	AverageVolume float64
	MACDValues    []float64
	RSI14Values   []float64
}

// LongerTermData 长期数据(4小时时间框架)
type LongerTermData struct {
	EMA20         float64
	EMA50         float64
	ATR3          float64
	ATR14         float64
	CurrentVolume float64
	AverageVolume float64
	MACDValues    []float64
	RSI14Values   []float64
}

// Binance API 响应结构
type ExchangeInfo struct {
	Symbols []SymbolInfo `json:"symbols"`
}

type SymbolInfo struct {
	Symbol            string `json:"symbol"`
	Status            string `json:"status"`
	BaseAsset         string `json:"baseAsset"`
	QuoteAsset        string `json:"quoteAsset"`
	ContractType      string `json:"contractType"`
	PricePrecision    int    `json:"pricePrecision"`
	QuantityPrecision int    `json:"quantityPrecision"`
}

type Kline struct {
	OpenTime            int64   `json:"openTime"`
	Open                float64 `json:"open"`
	High                float64 `json:"high"`
	Low                 float64 `json:"low"`
	Close               float64 `json:"close"`
	Volume              float64 `json:"volume"`
	CloseTime           int64   `json:"closeTime"`
	QuoteVolume         float64 `json:"quoteVolume"`
	Trades              int     `json:"trades"`
	TakerBuyBaseVolume  float64 `json:"takerBuyBaseVolume"`
	TakerBuyQuoteVolume float64 `json:"takerBuyQuoteVolume"`
}

type KlineResponse []interface{}

type PriceTicker struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

type Ticker24hr struct {
	Symbol             string `json:"symbol"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
}

// LongShortRatioData 多空持仓比数据
type LongShortRatioData struct {
	Symbol    string  `json:"symbol"`
	LongShort float64 `json:"longShort"` // 多空比，>1表示多头占优，<1表示空头占优
	Period    string  `json:"period"`    // 时间周期：5m, 15m, 30m, 1h, 4h, 1d
	Timestamp int64   `json:"timestamp"`
}

// Ticker24hrData 24小时统计数据
type Ticker24hrData struct {
	Symbol                string  `json:"symbol"`
	PriceChange           float64 `json:"priceChange"`           // 24小时价格变动
	PriceChangePercent    float64 `json:"priceChangePercent"`    // 24小时价格变动百分比
	Volume                float64 `json:"volume"`                // 24小时成交量
	QuoteVolume           float64 `json:"quoteVolume"`            // 24小时成交额
	HighPrice             float64 `json:"highPrice"`             // 24小时最高价
	LowPrice              float64 `json:"lowPrice"`              // 24小时最低价
	TakerBuyBaseVolume    float64 `json:"takerBuyBaseVolume"`    // 24小时主动买入量
	TakerBuyQuoteVolume   float64 `json:"takerBuyQuoteVolume"`   // 24小时主动买入成交额
	TakerSellBaseVolume   float64 `json:"takerSellBaseVolume"`   // 24小时主动卖出量
	TakerSellQuoteVolume  float64 `json:"takerSellQuoteVolume"` // 24小时主动卖出成交额
	BuySellRatio          float64 `json:"buySellRatio"`           // 主动买卖比 = 主动买入量/主动卖出量
}

// 特征数据结构
type SymbolFeatures struct {
	Symbol           string    `json:"symbol"`
	Timestamp        time.Time `json:"timestamp"`
	Price            float64   `json:"price"`
	PriceChange15Min float64   `json:"price_change_15min"`
	PriceChange1H    float64   `json:"price_change_1h"`
	PriceChange4H    float64   `json:"price_change_4h"`
	Volume           float64   `json:"volume"`
	VolumeRatio5     float64   `json:"volume_ratio_5"`
	VolumeRatio20    float64   `json:"volume_ratio_20"`
	VolumeTrend      float64   `json:"volume_trend"`
	RSI14            float64   `json:"rsi_14"`
	SMA5             float64   `json:"sma_5"`
	SMA10            float64   `json:"sma_10"`
	SMA20            float64   `json:"sma_20"`
	HighLowRatio     float64   `json:"high_low_ratio"`
	Volatility20     float64   `json:"volatility_20"`
	PositionInRange  float64   `json:"position_in_range"`
}

// 警报数据结构
type Alert struct {
	Type      string    `json:"type"`
	Symbol    string    `json:"symbol"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type Config struct {
	AlertThresholds AlertThresholds `json:"alert_thresholds"`
	UpdateInterval  int             `json:"update_interval"` // seconds
	CleanupConfig   CleanupConfig   `json:"cleanup_config"`
}

type AlertThresholds struct {
	VolumeSpike      float64 `json:"volume_spike"`
	PriceChange15Min float64 `json:"price_change_15min"`
	VolumeTrend      float64 `json:"volume_trend"`
	RSIOverbought    float64 `json:"rsi_overbought"`
	RSIOversold      float64 `json:"rsi_oversold"`
}
type CleanupConfig struct {
	InactiveTimeout   time.Duration `json:"inactive_timeout"`    // 不活跃超时时间
	MinScoreThreshold float64       `json:"min_score_threshold"` // 最低评分阈值
	NoAlertTimeout    time.Duration `json:"no_alert_timeout"`    // 无警报超时时间
	CheckInterval     time.Duration `json:"check_interval"`      // 检查间隔
}

var config = Config{
	AlertThresholds: AlertThresholds{
		VolumeSpike:      3.0,
		PriceChange15Min: 0.05,
		VolumeTrend:      2.0,
		RSIOverbought:    70,
		RSIOversold:      30,
	},
	CleanupConfig: CleanupConfig{
		InactiveTimeout:   30 * time.Minute,
		MinScoreThreshold: 15.0,
		NoAlertTimeout:    20 * time.Minute,
		CheckInterval:     5 * time.Minute,
	},
	UpdateInterval: 60, // 1 minute
}
