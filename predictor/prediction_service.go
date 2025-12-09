package predictor

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// PredictionResult 预测结果（内部使用）
type PredictionResult struct {
	Timestamp              string    `json:"timestamp"`
	Trend                  string    `json:"trend"` // "up", "down", "sideways"
	MeanPrediction         []float64 `json:"mean_prediction"`
	ConfidenceIntervalLower []float64 `json:"confidence_interval_lower"`
	ConfidenceIntervalUpper []float64 `json:"confidence_interval_upper"`
	PriceRange             []float64 `json:"price_range"`
	TrendStrength          float64   `json:"trend_strength"`
	PredictionHorizon      int       `json:"prediction_horizon"`
	NSimulations           int       `json:"n_simulations"`
	Error                  string    `json:"error,omitempty"`
}

// PredictionData krnos模型预测数据（对外接口）
type PredictionData struct {
	Trend                  string    `json:"trend"` // "up", "down", "sideways"
	TrendStrength          float64   `json:"trend_strength"`
	MeanPrediction         []float64 `json:"mean_prediction"`
	ConfidenceIntervalLower []float64 `json:"confidence_interval_lower"`
	ConfidenceIntervalUpper []float64 `json:"confidence_interval_upper"`
	PriceRange             []float64 `json:"price_range"`
	Timestamp              string    `json:"timestamp"`
}

// KrnosPredictorService krnos预测服务
type KrnosPredictorService struct {
	pythonScriptPath string
	cacheDir         string
	lastPrediction   map[string]*PredictionResult // 按币种存储的预测结果
	lastPredictionTime map[string]time.Time // 按币种存储的预测时间
	predictionInterval time.Duration // 30分钟
	lastPredictionPrice map[string]float64 // 按币种存储的上次预测时的价格（用于检测价格变动）
}

// NewKrnosPredictorService 创建新的预测服务
func NewKrnosPredictorService() *KrnosPredictorService {
	// 获取当前工作目录（在Docker容器中应该是/app）
	workDir, err := os.Getwd()
	if err != nil {
		workDir = "." // 如果获取失败，使用当前目录
		log.Printf("⚠️  无法获取工作目录，使用当前目录: %v", err)
	}
	
	scriptPath := filepath.Join(workDir, "predictor", "krnos_predictor.py")
	cacheDir := filepath.Join(workDir, "predictor_cache")
	
	// 确保缓存目录是绝对路径
	cacheDirAbs, err := filepath.Abs(cacheDir)
	if err != nil {
		cacheDirAbs = cacheDir // 如果转换失败，使用原路径
		log.Printf("⚠️  无法获取缓存目录绝对路径，使用相对路径: %v", err)
	}
	
	// 确保缓存目录存在
	if err := os.MkdirAll(cacheDirAbs, 0755); err != nil {
		log.Printf("⚠️  创建预测缓存目录失败: %v (路径: %s)", err, cacheDirAbs)
	} else {
		log.Printf("✓ 预测缓存目录已创建/确认存在: %s", cacheDirAbs)
	}
	
	return &KrnosPredictorService{
		pythonScriptPath: scriptPath,
		cacheDir:         cacheDirAbs, // 使用绝对路径
		predictionInterval: 30 * time.Minute,
		lastPrediction:    make(map[string]*PredictionResult),
		lastPredictionTime: make(map[string]time.Time),
		lastPredictionPrice: make(map[string]float64),
	}
}
// ShouldRepredict 判断是否需要重新预测（按币种）
func (s *KrnosPredictorService) ShouldRepredict(symbol string, currentPrice float64, currentTrend string) bool {
	// 如果该币种没有预测记录，需要重新预测
	lastTime, hasPrediction := s.lastPredictionTime[symbol]
	if !hasPrediction || lastTime.IsZero() {
		return true
	}

	// 如果距离上次预测超过30分钟，需要重新预测
	if time.Since(lastTime) >= s.predictionInterval {
		log.Printf("%s 距离上次预测已超过30分钟，需要重新预测", symbol)
		return true
	}

	// 如果预测结果与当前市场有较大差异，需要重新预测
	if lastPred, ok := s.lastPrediction[symbol]; ok && lastPred != nil {
		// 检查价格是否超出预测范围
		if len(lastPred.PriceRange) >= 2 {
			minPrice := lastPred.PriceRange[0]
			maxPrice := lastPred.PriceRange[1]
			if currentPrice < minPrice*0.95 || currentPrice > maxPrice*1.05 {
				log.Printf("%s 当前价格 %.2f 超出预测范围 [%.2f, %.2f]，需要重新预测", symbol, currentPrice, minPrice, maxPrice)
				return true
			}
		}

		// 检查价格变动幅度（如果价格变动超过5%，需要重新预测）
		if lastPrice, ok := s.lastPredictionPrice[symbol]; ok && lastPrice > 0 {
			priceChangePct := (currentPrice - lastPrice) / lastPrice * 100
			if priceChangePct < 0 {
				priceChangePct = -priceChangePct
			}
			if priceChangePct > 5.0 {
				log.Printf("%s 价格变动幅度 %.2f%% 超过5%%，需要重新预测（上次: %.2f, 当前: %.2f）", symbol, priceChangePct, lastPrice, currentPrice)
				return true
			}
		}

		// 检查趋势是否相反
		if (lastPred.Trend == "up" && currentTrend == "down") ||
			(lastPred.Trend == "down" && currentTrend == "up") {
			log.Printf("%s 预测趋势 %s 与当前趋势 %s 相反，需要重新预测", symbol, lastPred.Trend, currentTrend)
			return true
		}
	}

	return false
}
// nSimulations: 蒙特卡罗模拟次数（建议不超过10次以降低GPU压力和预测时间）
// predictionHorizon: 预测步数（标准为120步）
func (s *KrnosPredictorService) Predict(
	symbol string,
	currentPrice float64,
	priceHistory []float64,
	volumeHistory []float64,
	nSimulations int,
	predictionHorizon int,
) (*PredictionResult, error) {
	// 限制蒙特卡罗模拟次数不超过10次以降低GPU压力和预测时间
	if nSimulations > 10 {
		log.Printf("⚠️  蒙特卡罗模拟次数 %d 超过限制，已调整为10次", nSimulations)
		nSimulations = 10
	}
	// 准备输入数据
	inputData := map[string]interface{}{
		"price_history":     priceHistory,
		"volume_history":    volumeHistory,
		"n_simulations":     nSimulations,
		"prediction_horizon": predictionHorizon,
	}
	
	inputJSON, err := json.Marshal(inputData)
	if err != nil {
		return nil, fmt.Errorf("序列化输入数据失败: %w", err)
	}
	
	// 确保缓存目录存在
	if err := os.MkdirAll(s.cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %w", err)
	}
	
	// 创建临时输入文件（s.cacheDir已经是绝对路径）
	inputFileAbs := filepath.Join(s.cacheDir, "input.json")
	
	// 确保缓存目录存在（双重检查）
	if err := os.MkdirAll(s.cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %w (路径: %s)", err, s.cacheDir)
	}
	
	// 写入文件
	if err := os.WriteFile(inputFileAbs, inputJSON, 0644); err != nil {
		return nil, fmt.Errorf("写入输入文件失败: %w (路径: %s)", err, inputFileAbs)
	}
	
	// 验证文件是否存在（写入后立即检查）
	if _, err := os.Stat(inputFileAbs); os.IsNotExist(err) {
		return nil, fmt.Errorf("输入文件不存在（写入后检查）: %s", inputFileAbs)
	}
	
	// 记录文件创建成功（用于调试）
	log.Printf("✓ 输入文件已创建: %s (大小: %d 字节)", inputFileAbs, len(inputJSON))
	
	defer func() {
		// 清理文件
		if err := os.Remove(inputFileAbs); err != nil {
			log.Printf("⚠️  删除临时输入文件失败: %v (路径: %s)", err, inputFileAbs)
		}
	}()
	
	// 运行Python脚本（使用绝对路径）
	cmd := exec.Command("python3", s.pythonScriptPath, "--input", inputFileAbs)
	// 分离stdout和stderr：stdout用于JSON输出，stderr用于日志
	output, err := cmd.Output() // 只捕获stdout
	if err != nil {
		// 如果命令失败，尝试获取stderr中的错误信息
		if exitError, ok := err.(*exec.ExitError); ok {
			stderr := string(exitError.Stderr)
			return nil, fmt.Errorf("运行预测脚本失败: %w, stderr: %s, stdout: %s", err, stderr, string(output))
		}
		return nil, fmt.Errorf("运行预测脚本失败: %w, 输出: %s", err, string(output))
	}
	
	// 解析输出（只解析stdout中的JSON）
	var result PredictionResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("解析预测结果失败: %w, 输出内容: %s", err, string(output))
	}
	
	// 检查是否有错误字段
	if result.Error != "" {
		return nil, fmt.Errorf("预测脚本返回错误: %s", result.Error)
	}
	
	// 更新缓存（按币种存储）
	s.lastPrediction[symbol] = &result
	s.lastPredictionPrice[symbol] = currentPrice
	s.lastPredictionTime[symbol] = time.Now()
	
	// 保存到文件（按币种）
	latestFile := filepath.Join(s.cacheDir, fmt.Sprintf("latest_prediction_%s.json", symbol))
	if cacheData, err := json.MarshalIndent(result, "", "  "); err == nil {
		if err := os.WriteFile(latestFile, cacheData, 0644); err != nil {
			log.Printf("⚠️  %s 保存预测缓存失败: %v", symbol, err)
		}
	}
	
	return &result, nil
}

// GetLatestPrediction 获取最新预测结果（按币种）
func (s *KrnosPredictorService) GetLatestPrediction(symbol string) (*PredictionResult, error) {
	// 优先使用内存中的缓存
	if lastPred, ok := s.lastPrediction[symbol]; ok && lastPred != nil {
		// 验证预测数据是否合理（检查价格范围是否与当前币种匹配）
		// 这是一个额外的安全检查，防止使用错误的预测数据
		if len(lastPred.PriceRange) >= 2 {
			// 对于BTC/ETH，价格应该在合理范围内（>1000）
			// 对于其他币种，价格可能在0.001-1000之间
			// 这里只做基本验证，如果价格范围明显不合理，记录警告
			minPrice := lastPred.PriceRange[0]
			maxPrice := lastPred.PriceRange[1]
			if (symbol == "BTCUSDT" || symbol == "ETHUSDT") && (minPrice < 100 || maxPrice < 100) {
				log.Printf("⚠️  %s 缓存的预测价格范围 [%.2f, %.2f] 明显不合理，可能使用了错误的预测数据，将重新预测", symbol, minPrice, maxPrice)
				// 删除错误的缓存
				delete(s.lastPrediction, symbol)
				// 继续执行文件读取或重新预测
			} else {
				return lastPred, nil
			}
		} else {
			return lastPred, nil
		}
	}
	
	// 如果内存中没有，尝试从文件读取
	latestFile := filepath.Join(s.cacheDir, fmt.Sprintf("latest_prediction_%s.json", symbol))
	
	data, err := os.ReadFile(latestFile)
	if err != nil {
		// ⚠️ 重要：不再读取旧的全局文件，避免不同币种之间的数据混淆
		// 如果按币种的文件不存在，直接返回错误，让系统重新预测
		return nil, fmt.Errorf("读取%s的预测结果失败（文件不存在，需要重新预测）: %w", symbol, err)
	}
	
	var result PredictionResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析预测结果失败: %w", err)
	}
	
	// 验证预测数据是否合理（检查价格范围是否与当前币种匹配）
	if len(result.PriceRange) >= 2 {
		minPrice := result.PriceRange[0]
		maxPrice := result.PriceRange[1]
		// 对于BTC/ETH，价格应该在合理范围内（>1000）
		// 对于其他币种，价格可能在0.001-1000之间
		if (symbol == "BTCUSDT" || symbol == "ETHUSDT") && (minPrice < 100 || maxPrice < 100) {
			log.Printf("⚠️  %s 从文件读取的预测价格范围 [%.2f, %.2f] 明显不合理，可能使用了错误的预测数据，将重新预测", symbol, minPrice, maxPrice)
			return nil, fmt.Errorf("%s 预测数据不合理（价格范围 [%.2f, %.2f] 与币种不匹配），需要重新预测", symbol, minPrice, maxPrice)
		}
	}
	
	// 将结果缓存到内存中
	s.lastPrediction[symbol] = &result
	
	return &result, nil
}

// GetPredictionTrend 获取预测趋势（用于策略判断，按币种）
func (s *KrnosPredictorService) GetPredictionTrend(symbol string) (string, float64, error) {
	prediction, err := s.GetLatestPrediction(symbol)
	if err != nil {
		return "", 0, err
	}
	
	return prediction.Trend, prediction.TrendStrength, nil
}

