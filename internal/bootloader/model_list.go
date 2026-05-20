package bootloader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	DeepSeek   string = "DeepSeek"
	Kimi       string = "Kimi"
	XiaomiMimo string = "Xiaomi MiMo"
)

// ModelResponse 定义 API 响应结构（字段必须大写）
type ModelResponse struct {
	Data []ModelItem `json:"data"`
}

type ModelItem struct {
	ID string `json:"id"`
}

func getModelList(vendor string, apiKey string) ([]string, error) {
	var url string

	// ⭐ 使用 strings.ToLower 进行不区分大小写的比较
	switch vendor {
	case Kimi:
		url = "https://api.moonshot.cn/v1/models"
	case DeepSeek:
		url = "https://api.deepseek.com/models"
	case XiaomiMimo, "xiaomimimo":
		url = "https://api.xiaomimimo.com/v1/models"
	default:
		return nil, fmt.Errorf("unknown vendor: %s", vendor)
	}

	// 创建带超时的 HTTP 客户端
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	all, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var tmp ModelResponse
	err = json.Unmarshal(all, &tmp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	var modelList []string
	for _, item := range tmp.Data {
		if item.ID != "" {
			modelList = append(modelList, item.ID)
		}
	}

	if len(modelList) == 0 {
		return nil, fmt.Errorf("no models found in response")
	}

	return modelList, nil
}
