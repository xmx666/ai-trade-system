package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Provider AI提供商类型
type Provider string

const (
	ProviderDeepSeek Provider = "deepseek"
	ProviderQwen     Provider = "qwen"
	ProviderCustom   Provider = "custom"
)

// Client AI API配置
type Client struct {
	Provider   Provider
	APIKey     string
	BaseURL    string
	Model      string
	Timeout    time.Duration
	UseFullURL bool // 是否使用完整URL（不添加/chat/completions）
}

func New() *Client {
	// 默认配置
	return &Client{
		Provider: ProviderDeepSeek,
		BaseURL:  "https://api.deepseek.com/v1",
		Model:    "deepseek-chat",
		Timeout:  180 * time.Second, // 增加到180秒，因为AI需要分析大量数据，SiliconFlow等API可能响应较慢
	}
}

// SetDeepSeekAPIKey 设置DeepSeek API密钥
// customURL 为空时使用默认URL，customModel 为空时使用默认模型
func (client *Client) SetDeepSeekAPIKey(apiKey string, customURL string, customModel string) {
	client.Provider = ProviderDeepSeek
	client.APIKey = apiKey
	if customURL != "" {
		client.BaseURL = customURL
		log.Printf("🔧 [MCP] DeepSeek 使用自定义 BaseURL: %s", customURL)
	} else {
		client.BaseURL = "https://api.deepseek.com/v1"
		log.Printf("🔧 [MCP] DeepSeek 使用默认 BaseURL: %s", client.BaseURL)
	}
	if customModel != "" {
		client.Model = customModel
		log.Printf("🔧 [MCP] DeepSeek 使用自定义 Model: %s", customModel)
	} else {
		client.Model = "deepseek-chat"
		log.Printf("🔧 [MCP] DeepSeek 使用默认 Model: %s", client.Model)
	}
	// 打印 API Key 的前后各4位用于验证
	if len(apiKey) > 8 {
		log.Printf("🔧 [MCP] DeepSeek API Key: %s...%s", apiKey[:4], apiKey[len(apiKey)-4:])
	}
}

// SetQwenAPIKey 设置阿里云Qwen API密钥
// customURL 为空时使用默认URL，customModel 为空时使用默认模型
func (client *Client) SetQwenAPIKey(apiKey string, customURL string, customModel string) {
	client.Provider = ProviderQwen
	client.APIKey = apiKey
	if customURL != "" {
		client.BaseURL = customURL
		log.Printf("🔧 [MCP] Qwen 使用自定义 BaseURL: %s", customURL)
	} else {
		client.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		log.Printf("🔧 [MCP] Qwen 使用默认 BaseURL: %s", client.BaseURL)
	}
	if customModel != "" {
		client.Model = customModel
		log.Printf("🔧 [MCP] Qwen 使用自定义 Model: %s", customModel)
	} else {
		client.Model = "qwen-plus" // 可选: qwen-turbo, qwen-plus, qwen-max
		log.Printf("🔧 [MCP] Qwen 使用默认 Model: %s", client.Model)
	}
	// 打印 API Key 的前后各4位用于验证
	if len(apiKey) > 8 {
		log.Printf("🔧 [MCP] Qwen API Key: %s...%s", apiKey[:4], apiKey[len(apiKey)-4:])
	}
}

// SetCustomAPI 设置自定义OpenAI兼容API
func (client *Client) SetCustomAPI(apiURL, apiKey, modelName string) {
	client.Provider = ProviderCustom
	client.APIKey = apiKey

	// 检查URL是否以#结尾，如果是则使用完整URL（不添加/chat/completions）
	if strings.HasSuffix(apiURL, "#") {
		client.BaseURL = strings.TrimSuffix(apiURL, "#")
		client.UseFullURL = true
	} else {
		client.BaseURL = apiURL
		client.UseFullURL = false
	}

	client.Model = modelName
	client.Timeout = 180 * time.Second // 增加到180秒，SiliconFlow等API可能响应较慢
}

// SetClient 设置完整的AI配置（高级用户）
func (client *Client) SetClient(Client Client) {
	if Client.Timeout == 0 {
		Client.Timeout = 180 * time.Second // 默认180秒，SiliconFlow等API可能响应较慢
	}
	client = &Client
}

// CallWithMessages 使用 system + user prompt 调用AI API（推荐）
// 返回: (content, finish_reason, error)
func (client *Client) CallWithMessages(systemPrompt, userPrompt string) (string, string, error) {
	if client.APIKey == "" {
		return "", "", fmt.Errorf("AI API密钥未设置，请先调用 SetDeepSeekAPIKey() 或 SetQwenAPIKey()")
	}

	// 重试配置
	maxRetries := 3
	var lastErr error
	var lastFinishReason string

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			fmt.Printf("⚠️  AI API调用失败，正在重试 (%d/%d)...\n", attempt, maxRetries)
		}

		result, finishReason, err := client.callOnce(systemPrompt, userPrompt)
		if err == nil {
			if attempt > 1 {
				fmt.Printf("✓ AI API重试成功\n")
			}
			return result, finishReason, nil
		}

		lastErr = err
		lastFinishReason = finishReason
		// 如果不是网络错误，不重试
		if !isRetryableError(err) {
			return "", lastFinishReason, err
		}

		// 重试前等待
		if attempt < maxRetries {
			waitTime := time.Duration(attempt) * 2 * time.Second
			fmt.Printf("⏳ 等待%v后重试...\n", waitTime)
			time.Sleep(waitTime)
		}
	}

	return "", lastFinishReason, fmt.Errorf("重试%d次后仍然失败: %w", maxRetries, lastErr)
}

// callOnce 单次调用AI API（内部使用）
// 返回: (content, finish_reason, error)
func (client *Client) callOnce(systemPrompt, userPrompt string) (string, string, error) {
	// 打印当前 AI 配置
	log.Printf("📡 [MCP] AI 请求配置:")
	log.Printf("   Provider: %s", client.Provider)
	log.Printf("   BaseURL: %s", client.BaseURL)
	log.Printf("   Model: %s", client.Model)
	log.Printf("   UseFullURL: %v", client.UseFullURL)
	if len(client.APIKey) > 8 {
		log.Printf("   API Key: %s...%s", client.APIKey[:4], client.APIKey[len(client.APIKey)-4:])
	}

	// 构建 messages 数组
	messages := []map[string]string{}

	// 如果有 system prompt，添加 system message
	if systemPrompt != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": systemPrompt,
		})
	}

	// 添加 user message
	messages = append(messages, map[string]string{
		"role":    "user",
		"content": userPrompt,
	})

	// 详细的token统计
	log.Printf("📊 [MCP] Token使用详情:")
	log.Printf("   System prompt: %d 字符 (估算 %d tokens)", len(systemPrompt), int(float64(len(systemPrompt))*1.5))
	log.Printf("   User prompt: %d 字符 (估算 %d tokens)", len(userPrompt), int(float64(len(userPrompt))*1.5))
	
	// 估算token数量（简单估算：中文字符按2 tokens计算，英文按1 token计算）
	// 这是一个粗略的估算，实际token数量可能有所不同
	totalChars := len(systemPrompt) + len(userPrompt)
	// 粗略估算：中文字符约2 tokens，英文约0.25 tokens，平均按1.5 tokens/字符估算
	estimatedInputTokens := int(float64(totalChars) * 1.5)
	maxOutputTokens := 8000
	messageFormatOverhead := 100 // 消息格式开销
	totalEstimatedTokens := estimatedInputTokens + maxOutputTokens + messageFormatOverhead
	
	// DeepSeek模型的最大上下文长度是131072 tokens
	maxContextLength := 131072
	
	log.Printf("   输入tokens: %d (估算)", estimatedInputTokens)
	log.Printf("   输出tokens: %d (max_tokens)", maxOutputTokens)
	log.Printf("   格式开销: %d (估算)", messageFormatOverhead)
	log.Printf("   总计: %d tokens / %d tokens (使用率: %.1f%%)", totalEstimatedTokens, maxContextLength, float64(totalEstimatedTokens)/float64(maxContextLength)*100)
	
	if totalEstimatedTokens > maxContextLength {
		log.Printf("⚠️  [MCP] 警告：估算的token数量 (%d) 超过模型最大上下文长度 (%d)", totalEstimatedTokens, maxContextLength)
		log.Printf("⚠️  [MCP] 建议：减少System prompt或User prompt的长度")
		// 不直接返回错误，而是继续尝试，让API返回具体错误信息
		// 因为我们的估算可能不准确
	}

	// 构建请求体
	requestBody := map[string]interface{}{
		"model":       client.Model,
		"messages":    messages,
		"temperature": 0.5, // 降低temperature以提高JSON格式稳定性
		"max_tokens":  8000, // 设置为8000，确保输出不超过API限制
	}

	// 注意：response_format 参数仅 OpenAI 支持，DeepSeek/Qwen 不支持
	// 我们通过强化 prompt 和后处理来确保 JSON 格式正确

	// 记录max_tokens配置，用于调试
	log.Printf("📡 [MCP] 请求配置 - max_tokens: %v", requestBody["max_tokens"])

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", "", fmt.Errorf("序列化请求失败: %w", err)
	}

	// 创建HTTP请求
	var url string
	if client.UseFullURL {
		// 使用完整URL，不添加/chat/completions
		url = client.BaseURL
	} else {
		// 默认行为：添加/chat/completions
		url = fmt.Sprintf("%s/chat/completions", client.BaseURL)
	}
	log.Printf("📡 [MCP] 请求 URL: %s", url)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// 根据不同的Provider设置认证方式
	switch client.Provider {
	case ProviderDeepSeek:
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
	case ProviderQwen:
		// 阿里云Qwen使用API-Key认证
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
		// 注意：如果使用的不是兼容模式，可能需要不同的认证方式
	default:
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
	}

	// 发送请求（使用context控制超时）
	ctx, cancel := context.WithTimeout(context.Background(), client.Timeout)
	defer cancel()
	
	req = req.WithContext(ctx)
	httpClient := &http.Client{Timeout: client.Timeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		// 检查是否是超时错误
		if ctx.Err() == context.DeadlineExceeded {
			return "", "", fmt.Errorf("发送请求失败: 请求超时（%v）: %w", client.Timeout, err)
		}
		return "", "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("API返回错误 (status %d): %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"` // 完成原因：stop（正常结束）、length（达到token限制）
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("解析响应失败: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", "", fmt.Errorf("API返回空响应")
	}

	finishReason := result.Choices[0].FinishReason
	// 记录API响应的详细信息
	log.Printf("📡 [MCP] API响应信息:")
	log.Printf("   finish_reason: %s", finishReason)
	log.Printf("   prompt_tokens: %d", result.Usage.PromptTokens)
	log.Printf("   completion_tokens: %d", result.Usage.CompletionTokens)
	log.Printf("   total_tokens: %d", result.Usage.TotalTokens)
	log.Printf("   请求的max_tokens: 8000")

	// 检查是否因为token限制被截断
	if finishReason == "length" {
		log.Printf("⚠️  [MCP] AI响应因达到max_tokens限制而被截断！")
		log.Printf("   finish_reason=%s, completion_tokens=%d, 请求的max_tokens=8000", finishReason, result.Usage.CompletionTokens)
		if result.Usage.CompletionTokens < 8000 {
			log.Printf("⚠️  [MCP] 警告：completion_tokens (%d) < 请求的max_tokens (8000)，可能是API实际限制更小！", result.Usage.CompletionTokens)
		}
	}

	return result.Choices[0].Message.Content, finishReason, nil
}

// isRetryableError 判断错误是否可重试
func isRetryableError(err error) bool {
	errStr := err.Error()
	// 网络错误、超时、EOF等可以重试
	retryableErrors := []string{
		"EOF",
		"timeout",
		"Timeout",
		"deadline exceeded",
		"context deadline",
		"connection reset",
		"connection refused",
		"temporary failure",
		"no such host",
		"i/o timeout",
	}
	for _, retryable := range retryableErrors {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(retryable)) {
			return true
		}
	}
	return false
}
