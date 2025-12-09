package predictor

import (
	"log"
	"sync"
)

var (
	globalPredictor *KrnosPredictorService
	predictorOnce   sync.Once
)

// GetGlobalPredictor 获取全局预测服务实例（单例模式）
// 在系统启动时自动初始化，确保整个系统只有一个预测服务实例
func GetGlobalPredictor() *KrnosPredictorService {
	predictorOnce.Do(func() {
		globalPredictor = NewKrnosPredictorService()
		log.Println("🔮 krnos预测服务已初始化（单例模式）")
	})
	return globalPredictor
}

// GetPredictionForSymbol 为指定币种获取预测数据
// 这是主要的集成接口，在decision/engine.go中调用
//
// Args:
//   - symbol: 币种符号，如 "BTCUSDT"
//   - currentPrice: 当前价格
//   - currentTrend: 当前趋势 ("up", "down", "sideways")
//   - priceHistory: 历史价格序列（需要从market包获取）
//   - volumeHistory: 历史成交量序列（需要从market包获取）
//
// Returns:
//   - PredictionData: 预测数据，包含趋势、价格范围、置信区间等（如果预测失败返回nil）
//   - error: 错误信息（如果预测失败，返回错误但不影响主系统运行）
//
// 注意：此函数设计为容错模式，即使预测失败也不会影响主系统运行
// 调用方应该检查返回值是否为nil，如果为nil则继续执行，只是没有预测数据
func GetPredictionForSymbol(
	symbol string,
	currentPrice float64,
	currentTrend string,
	priceHistory []float64,
	volumeHistory []float64,
) (*PredictionData, error) {
	// 使用defer recover确保即使发生panic也不会影响主系统
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠️  %s krnos预测发生panic（已恢复）: %v", symbol, r)
		}
	}()

	predictor := GetGlobalPredictor()
	if predictor == nil {
		log.Printf("⚠️  %s 预测服务未初始化，跳过预测", symbol)
		return nil, nil
	}

	// 检查是否需要重新预测（按币种）
	if !predictor.ShouldRepredict(symbol, currentPrice, currentTrend) {
		// 使用缓存的预测结果
		latest, err := predictor.GetLatestPrediction(symbol)
		if err == nil && latest != nil {
			return convertToPredictionData(latest), nil
		}
		// 如果获取缓存失败，继续执行预测
	}

	// 需要重新预测
	// 检查是否有足够的历史数据（标准要求至少100个数据点，但如果没有数据会尝试使用缓存）
	if priceHistory == nil || volumeHistory == nil {
		// 历史数据为空，尝试使用缓存的预测结果
		latest, err := predictor.GetLatestPrediction(symbol)
		if err == nil && latest != nil {
			log.Printf("ℹ️  %s 历史数据为空，使用缓存的预测结果", symbol)
			return convertToPredictionData(latest), nil
		}
		log.Printf("⚠️  %s 历史数据为空且无缓存，跳过预测", symbol)
		return nil, nil
	}
	
	// 检查数据点数量（标准要求至少100个，但为了容错，至少需要20个）
	minRequired := 20
	if len(priceHistory) < minRequired || len(volumeHistory) < minRequired {
		log.Printf("⚠️  %s 历史数据不足（价格: %d, 成交量: %d），至少需要 %d 条，跳过预测", 
			symbol, len(priceHistory), len(volumeHistory), minRequired)
		return nil, nil
	}
	
	// 如果数据点少于标准450步，记录警告但继续预测
	if len(priceHistory) < 450 || len(volumeHistory) < 450 {
		log.Printf("⚠️  %s 历史数据少于标准450步（价格: %d, 成交量: %d），可能影响预测准确性", 
			symbol, len(priceHistory), len(volumeHistory))
	}

	// 运行预测（使用10次蒙特卡罗模拟，标准预测120步）
	// 使用defer recover确保预测过程中的panic不会影响主系统
	result, err := func() (*PredictionResult, error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("⚠️  %s 预测过程中发生panic（已恢复）: %v", symbol, r)
			}
		}()
		return predictor.Predict(symbol,
			currentPrice, 
			priceHistory,
			volumeHistory,
			10,   // 蒙特卡罗模拟次数（不超过10次）
			120,  // 预测未来120步（标准预测方式）
		)
	}()

	if err != nil {
		// 预测失败，只记录日志，不返回错误（容错模式）
		log.Printf("⚠️  %s krnos预测失败（不影响主系统）: %v", symbol, err)
		return nil, nil // 返回nil而不是error，确保调用方可以继续执行
	}

	if result == nil {
		log.Printf("⚠️  %s krnos预测返回空结果（不影响主系统）", symbol)
		return nil, nil
	}

	return convertToPredictionData(result), nil
}

// convertToPredictionData 将PredictionResult转换为PredictionData
func convertToPredictionData(result *PredictionResult) *PredictionData {
	return &PredictionData{
		Trend:                  result.Trend,
		TrendStrength:          result.TrendStrength,
		MeanPrediction:         result.MeanPrediction,
		ConfidenceIntervalLower: result.ConfidenceIntervalLower,
		ConfidenceIntervalUpper: result.ConfidenceIntervalUpper,
		PriceRange:             result.PriceRange,
		Timestamp:              result.Timestamp,
	}
}

