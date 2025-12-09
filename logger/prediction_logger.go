package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PredictionValidationLog 预测验证日志
type PredictionValidationLog struct {
	Timestamp      time.Time `json:"timestamp"`
	Symbol         string    `json:"symbol"`
	Side           string    `json:"side"` // "long" or "short"
	EntryPrice     float64   `json:"entry_price"`
	CurrentPrice   float64   `json:"current_price"`
	PredictionTime time.Time `json:"prediction_time"` // 预测生成时间
	CurrentIndex   int       `json:"current_index"`  // 当前预测数据点索引
	PredictedValue float64   `json:"predicted_value"` // 当前时间点的预测值
	RealValue      float64   `json:"real_value"`      // 当前真实价格
	Deviation      float64   `json:"deviation"`       // 偏差百分比
	TrendMatch     bool      `json:"trend_match"`     // 趋势是否匹配
	CycleNumber    int       `json:"cycle_number"`
}

// PredictionLogger 预测验证日志记录器
type PredictionLogger struct {
	logDir string
}

// NewPredictionLogger 创建预测验证日志记录器
func NewPredictionLogger(logDir string) *PredictionLogger {
	if logDir == "" {
		logDir = "prediction_logs"
	}
	
	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("⚠️ 创建预测日志目录失败: %v\n", err)
	}
	
	return &PredictionLogger{
		logDir: logDir,
	}
}

// LogPredictionValidation 记录预测验证日志
func (l *PredictionLogger) LogPredictionValidation(
	symbol string,
	side string,
	entryPrice float64,
	currentPrice float64,
	predictionTime time.Time,
	currentIndex int,
	predictedValue float64,
	cycleNumber int,
) error {
	// 计算偏差
	deviation := 0.0
	if predictedValue > 0 {
		deviation = ((currentPrice - predictedValue) / predictedValue) * 100
	}
	
	// 判断趋势是否匹配（简化版：如果偏差在±5%内认为匹配）
	trendMatch := deviation >= -5.0 && deviation <= 5.0
	
	log := PredictionValidationLog{
		Timestamp:      time.Now(),
		Symbol:         symbol,
		Side:           side,
		EntryPrice:     entryPrice,
		CurrentPrice:   currentPrice,
		PredictionTime: predictionTime,
		CurrentIndex:   currentIndex,
		PredictedValue: predictedValue,
		RealValue:      currentPrice,
		Deviation:      deviation,
		TrendMatch:     trendMatch,
		CycleNumber:    cycleNumber,
	}
	
	// 文件名：prediction_validation_SYMBOL_SIDE_TIMESTAMP.json
	filename := fmt.Sprintf("prediction_validation_%s_%s_%s.json",
		symbol, side, time.Now().Format("20060102_150405"))
	filepath := filepath.Join(l.logDir, filename)
	
	// 写入JSON文件
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化日志失败: %w", err)
	}
	
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("写入日志文件失败: %w", err)
	}
	
	return nil
}
