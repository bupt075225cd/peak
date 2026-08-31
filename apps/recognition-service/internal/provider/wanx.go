package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// wanxDefaultModel 通义万相图像编辑模型名。
	wanxDefaultModel = "wanx2.1-imageedit"
	// wanxDefaultEndpoint wanx 接口基地址（官方，含 /api/v1 前缀）。
	wanxDefaultEndpoint = "https://dashscope.aliyuncs.com/api/v1"
	// wanxPollInterval 轮询任务结果的间隔。
	wanxPollInterval = 2 * time.Second
)

// WanxClient 封装通义万相图像编辑（wanx2.1-imageedit）的异步任务调用：
// 提交任务 -> 轮询结果 -> 下载结果图。用于手写擦除等图像编辑能力。
type WanxClient struct {
	apiKey   string
	model    string
	endpoint string // 基地址，如 https://dashscope.aliyuncs.com/api/v1
	client   *http.Client
}

// NewWanxClient 创建 wanx 客户端。model/endpoint 为空时使用默认值。
func NewWanxClient(apiKey, model, endpoint string) *WanxClient {
	if model == "" {
		model = wanxDefaultModel
	}
	if endpoint == "" {
		endpoint = wanxDefaultEndpoint
	}
	return &WanxClient{
		apiKey:   apiKey,
		model:    model,
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// wanxSubmitRequest 图像编辑任务提交请求体。
type wanxSubmitRequest struct {
	Model string          `json:"model"`
	Input wanxSubmitInput `json:"input"`
}

// wanxSubmitInput 图像编辑输入参数。
type wanxSubmitInput struct {
	Function     string `json:"function"`
	Prompt       string `json:"prompt"`
	BaseImageURL string `json:"base_image_url"`
}

// wanxSubmitResponse 提交任务的响应体。
type wanxSubmitResponse struct {
	Output struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
	} `json:"output"`
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// wanxTaskResponse 查询任务结果的响应体。
type wanxTaskResponse struct {
	Output struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Results    []struct {
			URL     string `json:"url"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"results"`
	} `json:"output"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// EraseHandwriting 提交 wanx 图像编辑任务（description_edit 擦除手写），
// 轮询直到完成并下载结果图，返回结果图片字节。
func (w *WanxClient) EraseHandwriting(ctx context.Context, image []byte) ([]byte, error) {
	taskID, err := w.submit(ctx, image)
	if err != nil {
		return nil, err
	}
	resultURL, err := w.wait(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return w.download(ctx, resultURL)
}

// submit 提交图像编辑任务，返回 task_id。
func (w *WanxClient) submit(ctx context.Context, image []byte) (string, error) {
	reqBody := wanxSubmitRequest{
		Model: w.model,
		Input: wanxSubmitInput{
			Function:     "description_edit",
			Prompt:       "去除图片中的手写笔迹和批注痕迹，保留原有的印刷几何图形、线条与文字标注，不要改变图形结构",
			BaseImageURL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image),
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("wanx submit marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		w.endpoint+"/services/aigc/image2image/image-synthesis", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("wanx submit new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+w.apiKey)
	req.Header.Set("X-DashScope-Async", "enable")

	resp, err := w.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("wanx submit request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("wanx submit read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("wanx submit status %d: %s", resp.StatusCode, string(data))
	}

	var out wanxSubmitResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("wanx submit parse: %w", err)
	}
	if out.Output.TaskID == "" {
		return "", fmt.Errorf("wanx submit empty task id (code=%s message=%s)", out.Code, out.Message)
	}
	return out.Output.TaskID, nil
}

// wait 轮询任务结果直到 SUCCEEDED，返回结果图 URL；FAILED/CANCELED/UNKNOWN 返回错误。
func (w *WanxClient) wait(ctx context.Context, taskID string) (string, error) {
	ticker := time.NewTicker(wanxPollInterval)
	defer ticker.Stop()

	for {
		resp, err := w.getTask(ctx, taskID)
		if err != nil {
			return "", err
		}
		switch resp.Output.TaskStatus {
		case "SUCCEEDED":
			if len(resp.Output.Results) == 0 {
				return "", fmt.Errorf("wanx task %s succeeded but no results", taskID)
			}
			r := resp.Output.Results[0]
			if r.URL == "" {
				return "", fmt.Errorf("wanx task %s result url empty (code=%s message=%s)", taskID, r.Code, r.Message)
			}
			return r.URL, nil
		case "FAILED", "CANCELED":
			return "", fmt.Errorf("wanx task %s status=%s (code=%s message=%s)", taskID, resp.Output.TaskStatus, resp.Code, resp.Message)
		case "UNKNOWN":
			return "", fmt.Errorf("wanx task %s unknown", taskID)
		}

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("wanx wait: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// getTask 查询单个任务状态。
func (w *WanxClient) getTask(ctx context.Context, taskID string) (*wanxTaskResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.endpoint+"/tasks/"+taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("wanx get task new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+w.apiKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wanx get task request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wanx get task read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wanx get task status %d: %s", resp.StatusCode, string(data))
	}

	var out wanxTaskResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("wanx get task parse: %w", err)
	}
	return &out, nil
}

// download 下载结果图 URL，返回图片字节。
func (w *WanxClient) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("wanx download new request: %w", err)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wanx download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wanx download status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wanx download read: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("wanx download empty")
	}
	return data, nil
}
