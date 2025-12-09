package main

import (
	"encoding/json"
	"fmt"
	"log"
	"nofx/market"
	"os"
	"strings"
)

// 测试参数
const (
	HistorySteps = 450  // 历史数据步数（标准）
	Symbol       = "BTCUSDT" // 测试币种
	Interval     = "3m"      // K线间隔
)

func main() {
	log.Println("🔮 krnos预测服务真实数据测试")
	log.Println(strings.Repeat("=", 60))
	log.Printf("测试参数:")
	log.Printf("  币种: %s", Symbol)
	log.Printf("  K线间隔: %s", Interval)
	log.Printf("  历史数据步数: %d", HistorySteps)
	log.Println(strings.Repeat("=", 60))

	// 1. 测试历史数据客户端
	log.Println("\n测试1: 从币安获取真实历史数据")
	historyClient := market.NewHistoryClient()

	klines, err := historyClient.GetLatestKlinesForPrediction(Symbol, Interval, HistorySteps)
	if err != nil {
		log.Fatalf("❌ 获取历史数据失败: %v", err)
	}

	log.Printf("✓ 成功获取 %d 条K线数据", len(klines))
	if len(klines) > 0 {
		latest := klines[len(klines)-1]
		log.Printf("  最新K线时间: %v", latest.OpenTime)
		log.Printf("  最新价格: %.2f", latest.Close)
		log.Printf("  最新成交量: %.2f", latest.Volume)
	}

	// 2. 提取价格和成交量序列
	log.Println("\n测试2: 提取价格和成交量序列")
	priceHistory := make([]float64, len(klines))
	volumeHistory := make([]float64, len(klines))

	for i, kline := range klines {
		priceHistory[i] = kline.Close
		volumeHistory[i] = kline.Volume
	}

	log.Printf("✓ 价格序列: %d 条", len(priceHistory))
	log.Printf("  价格范围: [%.2f, %.2f]", min(priceHistory), max(priceHistory))
	log.Printf("✓ 成交量序列: %d 条", len(volumeHistory))
	log.Printf("  成交量范围: [%.2f, %.2f]", min(volumeHistory), max(volumeHistory))

	// 3. 验证数据
	log.Println("\n测试3: 验证数据")
	if len(priceHistory) < 100 {
		log.Fatalf("❌ 数据不足: 只有 %d 条，至少需要 100 条", len(priceHistory))
	}
	if len(priceHistory) != len(volumeHistory) {
		log.Fatalf("❌ 数据长度不一致: 价格 %d 条, 成交量 %d 条", len(priceHistory), len(volumeHistory))
	}
	log.Println("✓ 数据验证通过")

	// 4. 保存数据到JSON文件供Python脚本使用
	log.Println("\n测试4: 保存数据到JSON文件")
	data := map[string]interface{}{
		"symbol":         Symbol,
		"interval":       Interval,
		"price_history":  priceHistory,
		"volume_history": volumeHistory,
		"history_steps":  HistorySteps,
		"prediction_steps": 120,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Fatalf("❌ 序列化数据失败: %v", err)
	}

	outputFile := "test_data.json"
	if err := os.WriteFile(outputFile, jsonData, 0644); err != nil {
		log.Fatalf("❌ 写入文件失败: %v", err)
	}

	log.Printf("✓ 数据已保存到 %s", outputFile)
	log.Printf("  文件大小: %d 字节", len(jsonData))

	// 5. 输出摘要
	log.Println("\n" + strings.Repeat("=", 60))
	log.Println("测试总结")
	log.Println(strings.Repeat("=", 60))
	log.Println("✓ 所有测试通过")
	log.Printf("  数据文件: %s", outputFile)
	log.Println("  可以运行 python3 test_krnos.py 进行预测测试")
}

func min(slice []float64) float64 {
	if len(slice) == 0 {
		return 0
	}
	min := slice[0]
	for _, v := range slice[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func max(slice []float64) float64 {
	if len(slice) == 0 {
		return 0
	}
	max := slice[0]
	for _, v := range slice[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

