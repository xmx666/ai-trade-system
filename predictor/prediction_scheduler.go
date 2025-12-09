package predictor

import (
	"fmt"
	"log"
	"nofx/market"
	"time"
)

const (
	// krnos标准预测参数
	HistoryStepsForPrediction = 450  // 历史数据步数（标准450步）
	PredictionSteps = 120            // 预测步数（标准120步）
	// 预测所需的最小数据点数
	MinDataPointsForPrediction = 100
)

// PredictionScheduler 预测调度器，负责定期运行预测
type PredictionScheduler struct {
	predictor    *KrnosPredictorService
	interval     time.Duration // 预测更新间隔（30分钟）
	isRunning    bool
	stopChan     chan bool
	coinsToWatch []string // 需要预测的币种列表
}

// NewPredictionScheduler 创建预测调度器
func NewPredictionScheduler(coins []string) *PredictionScheduler {
	return &PredictionScheduler{
		predictor:    GetGlobalPredictor(),
		interval:     30 * time.Minute,
		isRunning:    false,
		stopChan:     make(chan bool),
		coinsToWatch: coins,
	}
}

// Start 启动预测调度器
// 立即运行一次预测，然后每30分钟更新一次
func (ps *PredictionScheduler) Start() {
	if ps.isRunning {
		log.Println("⚠️  预测调度器已在运行")
		return
	}

	ps.isRunning = true
	log.Println("🔮 启动krnos预测调度器...")
	log.Printf("   预测间隔: %v", ps.interval)
	log.Printf("   监控币种: %v", ps.coinsToWatch)

	// 立即运行第一次预测
	go ps.runPredictionOnce()

	// 启动定时器
	ticker := time.NewTicker(ps.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 定时触发预测
			go ps.runPredictionOnce()
		case <-ps.stopChan:
			log.Println("⏹  预测调度器已停止")
			return
		}
	}
}

// Stop 停止预测调度器
func (ps *PredictionScheduler) Stop() {
	if !ps.isRunning {
		return
	}
	ps.isRunning = false
	ps.stopChan <- true
}

// runPredictionOnce 运行一次预测（为所有监控的币种）
// 使用defer recover确保即使预测失败也不会影响主系统
func (ps *PredictionScheduler) runPredictionOnce() {
	// 顶层recover，确保整个预测流程的panic不会影响主系统
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠️  krnos预测调度器发生panic（已恢复，不影响主系统）: %v", r)
		}
	}()

	log.Println("🔮 开始运行krnos预测...")

	// 统计成功和失败的数量
	successCount := 0
	failureCount := 0
	totalCount := len(ps.coinsToWatch)

	for _, symbol := range ps.coinsToWatch {
		// 为每个币种的预测添加recover，确保单个币种失败不影响其他币种
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("⚠️  %s 预测过程中发生panic（已恢复，继续处理其他币种）: %v", symbol, r)
					failureCount++
				}
			}()
			
			// 获取市场数据
			data, err := market.Get(symbol)
			if err != nil {
				log.Printf("⚠️  %s 获取市场数据失败: %v", symbol, err)
				failureCount++
				return // 使用return而不是continue，因为我们在匿名函数中
			}

			// 从币安获取真实历史数据（标准450步，确保是最新实时数据）
			log.Printf("📊 %s 开始获取最新实时K线数据用于预测（标准450步）...", symbol)
			priceHistory, err := ps.getPriceHistory(symbol, HistoryStepsForPrediction)
			if err != nil {
				log.Printf("⚠️  %s 获取历史价格数据失败: %v", symbol, err)
				failureCount++
				return
			}
			if len(priceHistory) > 0 {
				log.Printf("✓ %s 获取到最新价格数据: %d 条，最新价格: %.2f", symbol, len(priceHistory), priceHistory[len(priceHistory)-1])
			}

			volumeHistory, err := ps.getVolumeHistory(symbol, HistoryStepsForPrediction)
			if err != nil {
				log.Printf("⚠️  %s 获取历史成交量数据失败: %v", symbol, err)
				failureCount++
				return
			}
			if len(volumeHistory) > 0 {
				log.Printf("✓ %s 获取到最新成交量数据: %d 条，最新成交量: %.2f", symbol, len(volumeHistory), volumeHistory[len(volumeHistory)-1])
			}

			// 验证数据是否足够
			if len(priceHistory) < MinDataPointsForPrediction || len(volumeHistory) < MinDataPointsForPrediction {
				log.Printf("⚠️  %s 历史数据不足（价格: %d, 成交量: %d），至少需要 %d 条，跳过预测", 
					symbol, len(priceHistory), len(volumeHistory), MinDataPointsForPrediction)
				failureCount++
				return
			}

			// 验证数据长度是否一致
			if len(priceHistory) != len(volumeHistory) {
				log.Printf("⚠️  %s 价格和成交量数据长度不一致（价格: %d, 成交量: %d），使用较短的长度", 
					symbol, len(priceHistory), len(volumeHistory))
				minLen := len(priceHistory)
				if len(volumeHistory) < minLen {
					minLen = len(volumeHistory)
				}
				priceHistory = priceHistory[:minLen]
				volumeHistory = volumeHistory[:minLen]
			}

			// 判断当前趋势
			currentTrend := ps.determineTrend(data)

			// 运行预测（标准120步预测）
			prediction, err := GetPredictionForSymbol(
				symbol,
				data.CurrentPrice,
				currentTrend,
				priceHistory,
				volumeHistory,
			)

			// 判断预测是否成功：必须没有错误且返回了有效的预测数据
			if err != nil {
				log.Printf("⚠️  %s 预测失败（不影响主系统）: %v", symbol, err)
				failureCount++
			} else if prediction == nil {
				log.Printf("⚠️  %s 预测返回空结果（不影响主系统）", symbol)
				failureCount++
			} else {
				log.Printf("✓ %s 预测完成", symbol)
				successCount++
			}
		}() // 结束defer recover的作用域
	}

	// 根据结果显示不同的消息
	if successCount > 0 {
		if failureCount > 0 {
			log.Printf("🔮 krnos预测运行完成: 成功 %d/%d，失败 %d/%d", successCount, totalCount, failureCount, totalCount)
		} else {
			log.Printf("🔮 krnos预测运行完成: 全部成功 (%d/%d)", successCount, totalCount)
		}
	} else {
		log.Printf("❌ krnos预测运行失败: 全部失败 (%d/%d)，请检查模型加载状态", failureCount, totalCount)
		log.Printf("   提示: 如果模型未加载，请检查Kronos项目是否正确复制到容器中")
	}
}

// getPriceHistory 从币安获取真实历史价格数据
func (ps *PredictionScheduler) getPriceHistory(symbol string, count int) ([]float64, error) {
	// 使用3分钟K线数据（与交易系统保持一致）
	historyClient := market.NewHistoryClient()
	
	klines, err := historyClient.GetLatestKlinesForPrediction(symbol, "3m", count)
	if err != nil {
		return nil, fmt.Errorf("获取历史K线数据失败: %w", err)
	}

	// 提取收盘价（Close）作为价格序列
	prices := make([]float64, len(klines))
	for i, kline := range klines {
		prices[i] = kline.Close
	}

	return prices, nil
}

// getVolumeHistory 从币安获取真实历史成交量数据
func (ps *PredictionScheduler) getVolumeHistory(symbol string, count int) ([]float64, error) {
	// 使用3分钟K线数据（与交易系统保持一致）
	historyClient := market.NewHistoryClient()
	
	klines, err := historyClient.GetLatestKlinesForPrediction(symbol, "3m", count)
	if err != nil {
		return nil, fmt.Errorf("获取历史K线数据失败: %w", err)
	}

	// 提取成交量（Volume）作为成交量序列
	volumes := make([]float64, len(klines))
	for i, kline := range klines {
		volumes[i] = kline.Volume
	}

	return volumes, nil
}

// determineTrend 判断当前趋势
func (ps *PredictionScheduler) determineTrend(data *market.Data) string {
	if data.PriceChange1h > 0.5 {
		return "up"
	} else if data.PriceChange1h < -0.5 {
		return "down"
	}
	return "sideways"
}

