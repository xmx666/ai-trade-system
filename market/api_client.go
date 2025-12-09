package market

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

const (
	baseURL = "https://fapi.binance.com"
)

type APIClient struct {
	client *http.Client
}

func NewAPIClient() *APIClient {
	// 配置HTTP客户端，支持代理（与WebSocket客户端保持一致）
	transport := &http.Transport{}
	
	// 从环境变量读取代理配置
	if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxy)
			log.Printf("🌐 使用代理连接HTTP: %s", proxyURL)
		}
	}
	
	return &APIClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

func (c *APIClient) GetExchangeInfo() (*ExchangeInfo, error) {
	url := fmt.Sprintf("%s/fapi/v1/exchangeInfo", baseURL)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var exchangeInfo ExchangeInfo
	err = json.Unmarshal(body, &exchangeInfo)
	if err != nil {
		return nil, err
	}

	return &exchangeInfo, nil
}

func (c *APIClient) GetKlines(symbol, interval string, limit int) ([]Kline, error) {
	url := fmt.Sprintf("%s/fapi/v1/klines", baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("symbol", symbol)
	q.Add("interval", interval)
	q.Add("limit", strconv.Itoa(limit))
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
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

func parseKline(kr KlineResponse) (Kline, error) {
	var kline Kline

	if len(kr) < 11 {
		return kline, fmt.Errorf("invalid kline data")
	}

	// 解析各个字段
	kline.OpenTime = int64(kr[0].(float64))
	kline.Open, _ = strconv.ParseFloat(kr[1].(string), 64)
	kline.High, _ = strconv.ParseFloat(kr[2].(string), 64)
	kline.Low, _ = strconv.ParseFloat(kr[3].(string), 64)
	kline.Close, _ = strconv.ParseFloat(kr[4].(string), 64)
	kline.Volume, _ = strconv.ParseFloat(kr[5].(string), 64)
	kline.CloseTime = int64(kr[6].(float64))
	kline.QuoteVolume, _ = strconv.ParseFloat(kr[7].(string), 64)
	kline.Trades = int(kr[8].(float64))
	kline.TakerBuyBaseVolume, _ = strconv.ParseFloat(kr[9].(string), 64)
	kline.TakerBuyQuoteVolume, _ = strconv.ParseFloat(kr[10].(string), 64)

	return kline, nil
}

func (c *APIClient) GetCurrentPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/fapi/v1/ticker/price", baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	q := req.URL.Query()
	q.Add("symbol", symbol)
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var ticker PriceTicker
	err = json.Unmarshal(body, &ticker)
	if err != nil {
		return 0, err
	}

	price, err := strconv.ParseFloat(ticker.Price, 64)
	if err != nil {
		return 0, err
	}

	return price, nil
}

// GetLongShortRatio 获取全市场多空持仓比
func (c *APIClient) GetLongShortRatio(symbol, period string) (*LongShortRatioData, error) {
	url := fmt.Sprintf("%s/fapi/v1/globalLongShortAccountRatio", baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("symbol", symbol)
	q.Add("period", period) // 5m, 15m, 30m, 1h, 4h, 1d
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Symbol    string  `json:"symbol"`
		LongShort float64 `json:"longShortRatio"`
		Timestamp int64   `json:"timestamp"`
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	return &LongShortRatioData{
		Symbol:    result.Symbol,
		LongShort: result.LongShort,
		Period:    period,
		Timestamp: result.Timestamp,
	}, nil
}

// GetTopTraderLongShortRatio 获取大户多空持仓比
func (c *APIClient) GetTopTraderLongShortRatio(symbol, period string) (*LongShortRatioData, error) {
	url := fmt.Sprintf("%s/fapi/v1/topLongShortAccountRatio", baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("symbol", symbol)
	q.Add("period", period) // 5m, 15m, 30m, 1h, 4h, 1d
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Symbol    string  `json:"symbol"`
		LongShort float64 `json:"longShortRatio"`
		Timestamp int64   `json:"timestamp"`
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	return &LongShortRatioData{
		Symbol:    result.Symbol,
		LongShort: result.LongShort,
		Period:    period,
		Timestamp: result.Timestamp,
	}, nil
}

// Get24hrTicker 获取24小时统计数据
func (c *APIClient) Get24hrTicker(symbol string) (*Ticker24hrData, error) {
	url := fmt.Sprintf("%s/fapi/v1/ticker/24hr", baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("symbol", symbol)
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Symbol                string `json:"symbol"`
		PriceChange           string `json:"priceChange"`
		PriceChangePercent    string `json:"priceChangePercent"`
		Volume                string `json:"volume"`
		QuoteVolume           string `json:"quoteVolume"`
		HighPrice             string `json:"highPrice"`
		LowPrice              string `json:"lowPrice"`
		TakerBuyBaseVolume    string `json:"takerBuyBaseVolume"`
		TakerBuyQuoteVolume   string `json:"takerBuyQuoteVolume"`
		TakerSellBaseVolume   string `json:"takerSellBaseVolume"`
		TakerSellQuoteVolume  string `json:"takerSellQuoteVolume"`
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	priceChange, _ := strconv.ParseFloat(result.PriceChange, 64)
	priceChangePercent, _ := strconv.ParseFloat(result.PriceChangePercent, 64)
	volume, _ := strconv.ParseFloat(result.Volume, 64)
	quoteVolume, _ := strconv.ParseFloat(result.QuoteVolume, 64)
	highPrice, _ := strconv.ParseFloat(result.HighPrice, 64)
	lowPrice, _ := strconv.ParseFloat(result.LowPrice, 64)
	takerBuyBaseVolume, _ := strconv.ParseFloat(result.TakerBuyBaseVolume, 64)
	takerBuyQuoteVolume, _ := strconv.ParseFloat(result.TakerBuyQuoteVolume, 64)
	takerSellBaseVolume, _ := strconv.ParseFloat(result.TakerSellBaseVolume, 64)
	takerSellQuoteVolume, _ := strconv.ParseFloat(result.TakerSellQuoteVolume, 64)

	buySellRatio := 1.0
	if takerSellBaseVolume > 0 {
		buySellRatio = takerBuyBaseVolume / takerSellBaseVolume
	}

	return &Ticker24hrData{
		Symbol:               result.Symbol,
		PriceChange:          priceChange,
		PriceChangePercent:   priceChangePercent,
		Volume:               volume,
		QuoteVolume:          quoteVolume,
		HighPrice:            highPrice,
		LowPrice:             lowPrice,
		TakerBuyBaseVolume:   takerBuyBaseVolume,
		TakerBuyQuoteVolume:  takerBuyQuoteVolume,
		TakerSellBaseVolume:  takerSellBaseVolume,
		TakerSellQuoteVolume: takerSellQuoteVolume,
		BuySellRatio:         buySellRatio,
	}, nil
}
