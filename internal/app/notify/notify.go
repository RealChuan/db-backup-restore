package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Notifier 通知器
type Notifier struct {
	webhookURL string
	client     *http.Client
}

// NewNotifier 创建通知器，校验 webhook URL 合法性
func NewNotifier(webhookURL string) *Notifier {
	return &Notifier{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// validateWebhookURL 校验 webhook URL，防止 SSRF 攻击。
// 校验规则：
//   - 仅允许 https scheme（生产环境安全要求）
//   - 拒绝解析到私有IP/回环地址的目标
func validateWebhookURL(ctx context.Context, webhookURL string) error {
	if webhookURL == "" {
		return nil
	}

	parsed, err := url.Parse(webhookURL)
	if err != nil {
		return fmt.Errorf("无效的 webhook URL: %w", err)
	}

	// 仅允许 https scheme
	if parsed.Scheme != "https" {
		return fmt.Errorf("webhook URL 必须使用 https 协议，当前: %s", parsed.Scheme)
	}

	// 拒绝解析到私有IP/回环地址
	host := parsed.Hostname()
	if host == "" {
		return errors.New("webhook URL 缺少主机名")
	}

	// 先检查是否是 IP 地址
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateOrLoopbackIP(ip) {
			return fmt.Errorf("webhook URL 不允许指向私有或回环地址: %s", host)
		}
		return nil
	}

	// 对于域名，使用带 context 的 DNS 解析后检查 IP
	resolver := net.Resolver{}
	ipAddrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("无法解析 webhook URL 主机名: %w", err)
	}
	for _, addr := range ipAddrs {
		if isPrivateOrLoopbackIP(addr.IP) {
			return fmt.Errorf("webhook URL 主机名解析到私有或回环地址: %s -> %s", host, addr.IP)
		}
	}

	return nil
}

// isPrivateOrLoopbackIP 判断 IP 是否为私有地址或回环地址
func isPrivateOrLoopbackIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsUnspecified() {
		return true
	}
	return false
}

// Notify 发送通知
func (n *Notifier) Notify(ctx context.Context, operation, dbType, status, message string) error {
	if n.webhookURL == "" {
		return nil
	}

	// 发送前校验 URL，防止 SSRF
	if err := validateWebhookURL(ctx, n.webhookURL); err != nil {
		return fmt.Errorf("webhook URL 校验失败: %w", err)
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

// String 返回脱敏后的 webhook URL
func (n *Notifier) String() string {
	if n.webhookURL == "" {
		return ""
	}
	parsed, err := url.Parse(n.webhookURL)
	if err != nil {
		return "***"
	}
	return parsed.Scheme + "://" + parsed.Host + "/***"
}
