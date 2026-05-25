package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Notifier 通知器
type Notifier struct {
	webhookURL string
	client     *http.Client
}

// NewNotifier 创建通知器
func NewNotifier(webhookURL string) *Notifier {
	return &Notifier{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Notify 发送通知
func (n *Notifier) Notify(ctx context.Context, operation, dbType, status, message string) error {
	if n.webhookURL == "" {
		return nil
	}

	payload := map[string]string{
		"operation": operation,
		"db_type":   dbType,
		"status":    status,
		"message":   message,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化通知内容失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建通知请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("发送通知失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("通知请求返回非成功状态码: %d", resp.StatusCode)
	}

	return nil
}
