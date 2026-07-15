package notify

import (
	"context"
	"net"
	"testing"
)

func TestIsPrivateOrLoopbackIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		// 回环地址
		{name: "IPv4回环地址_127.0.0.1", ip: "127.0.0.1", want: true},
		{name: "IPv4回环地址_127.0.0.2", ip: "127.0.0.2", want: true},
		{name: "IPv6回环地址", ip: "::1", want: true},

		// 私有地址
		{name: "IPv4私有_10.0.0.1", ip: "10.0.0.1", want: true},
		{name: "IPv4私有_10.255.255.255", ip: "10.255.255.255", want: true},
		{name: "IPv4私有_172.16.0.1", ip: "172.16.0.1", want: true},
		{name: "IPv4私有_172.31.255.255", ip: "172.31.255.255", want: true},
		{name: "IPv4私有_192.168.0.1", ip: "192.168.0.1", want: true},
		{name: "IPv4私有_192.168.1.1", ip: "192.168.1.1", want: true},
		{name: "IPv6私有_fdc0", ip: "fdc0:1234::1", want: true},
		{name: "IPv6私有_fc00", ip: "fc00::1", want: true},

		// 链路本地地址
		{name: "IPv4链路本地_169.254.0.1", ip: "169.254.0.1", want: true},
		{name: "IPv4链路本地_169.254.169.254", ip: "169.254.169.254", want: true},
		{name: "IPv6链路本地", ip: "fe80::1", want: true},

		// 未指定地址
		{name: "IPv4未指定地址_0.0.0.0", ip: "0.0.0.0", want: true},
		{name: "IPv6未指定地址", ip: "::", want: true},

		// 公网地址
		{name: "IPv4公网_8.8.8.8", ip: "8.8.8.8", want: false},
		{name: "IPv4公网_1.1.1.1", ip: "1.1.1.1", want: false},
		{name: "IPv4公网_203.0.113.1", ip: "203.0.113.1", want: false},
		{name: "IPv6公网_2001_db8", ip: "2001:db8::1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("无法解析测试 IP: %s", tt.ip)
			}
			got := isPrivateOrLoopbackIP(ip)
			if got != tt.want {
				t.Errorf("isPrivateOrLoopbackIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestValidateWebhookURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		// 空URL
		{name: "空URL_允许", url: "", wantErr: false},

		// scheme 校验
		{name: "http协议_拒绝", url: "http://example.com/webhook", wantErr: true},
		{name: "ftp协议_拒绝", url: "ftp://example.com/webhook", wantErr: true},
		{name: "无协议_拒绝", url: "example.com/webhook", wantErr: true},

		// https + 有效域名（仅校验格式，不做真实DNS解析）
		// 注意：这里使用公网域名，但测试中不依赖DNS解析成功

		// IP 字面量 - 私有/回环地址
		{name: "IPv4回环_拒绝", url: "https://127.0.0.1/webhook", wantErr: true},
		{name: "IPv4私有A类_拒绝", url: "https://10.0.0.1/webhook", wantErr: true},
		{name: "IPv4私有B类_拒绝", url: "https://172.16.0.1/webhook", wantErr: true},
		{name: "IPv4私有C类_拒绝", url: "https://192.168.1.1/webhook", wantErr: true},
		{name: "IPv4链路本地_拒绝", url: "https://169.254.169.254/webhook", wantErr: true},
		{name: "IPv4未指定地址_拒绝", url: "https://0.0.0.0/webhook", wantErr: true},
		{name: "IPv6回环_拒绝", url: "https://[::1]/webhook", wantErr: true},
		{name: "IPv6私有_拒绝", url: "https://[fc00::1]/webhook", wantErr: true},

		// IP 字面量 - 公网地址
		{name: "IPv4公网_允许", url: "https://8.8.8.8/webhook", wantErr: false},

		// 无效URL
		{name: "无效URL_拒绝", url: "://missing-scheme", wantErr: true},

		// 缺少主机名
		{name: "缺少主机名_拒绝", url: "https:///webhook", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateWebhookURL(context.Background(), tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWebhookURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateWebhookURL_DNSError(t *testing.T) {
	t.Parallel()

	// 使用一个不可能解析的域名，验证DNS解析失败时返回错误
	err := validateWebhookURL(context.Background(), "https://this-domain-does-not-exist-invalid-12345.example/webhook")
	if err == nil {
		t.Error("期望 DNS 解析失败返回错误，得到 nil")
	}
}

func TestNewNotifier(t *testing.T) {
	t.Parallel()

	t.Run("创建通知器_带URL", func(t *testing.T) {
		t.Parallel()
		n := NewNotifier("https://example.com/webhook")
		if n == nil {
			t.Fatal("NewNotifier 返回 nil")
		}
		if n.webhookURL != "https://example.com/webhook" {
			t.Errorf("webhookURL = %q, want %q", n.webhookURL, "https://example.com/webhook")
		}
		if n.client == nil {
			t.Error("client 不应为 nil")
		}
	})

	t.Run("创建通知器_空URL", func(t *testing.T) {
		t.Parallel()
		n := NewNotifier("")
		if n == nil {
			t.Fatal("NewNotifier 返回 nil")
		}
		if n.webhookURL != "" {
			t.Errorf("webhookURL = %q, want empty", n.webhookURL)
		}
	})
}

func TestNotifier_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		webhookURL string
		want       string
	}{
		{name: "空URL_返回空字符串", webhookURL: "", want: ""},
		{name: "有效URL_脱敏", webhookURL: "https://example.com/path/to/webhook?token=secret", want: "https://example.com/***"},
		{name: "有效URL_带端口_脱敏", webhookURL: "https://example.com:8443/webhook", want: "https://example.com:8443/***"},
		{name: "无效URL_返回星号", webhookURL: "://invalid", want: "***"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			n := NewNotifier(tt.webhookURL)
			got := n.String()
			if got != tt.want {
				t.Errorf("Notifier.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
