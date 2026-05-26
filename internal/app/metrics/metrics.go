package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// OperationMetric 操作指标
type OperationMetric struct {
	Operation  string        `json:"operation"`
	DBType     string        `json:"db_type"`
	Status     string        `json:"status"`
	Duration   time.Duration `json:"duration"`
	BackupSize int64         `json:"backup_size,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}

// Collector 指标收集器
type Collector struct {
	mu         sync.Mutex
	metrics    []OperationMetric
	maxMetrics int
}

// NewCollector 创建指标收集器
func NewCollector() *Collector {
	return &Collector{
		metrics:    make([]OperationMetric, 0),
		maxMetrics: 1000,
	}
}

// NewCollectorWithMax 创建指定最大容量的指标收集器
func NewCollectorWithMax(maxMetrics int) *Collector {
	return &Collector{
		metrics:    make([]OperationMetric, 0),
		maxMetrics: maxMetrics,
	}
}

// Record 记录操作指标
func (c *Collector) Record(m OperationMetric) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.metrics) >= c.maxMetrics {
		c.metrics = c.metrics[1:]
	}
	c.metrics = append(c.metrics, m)
}

// ExportJSON 导出为 JSON 格式
func (c *Collector) ExportJSON() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := json.MarshalIndent(c.metrics, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Summary 输出指标摘要
func (c *Collector) Summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := len(c.metrics)
	success := 0
	failed := 0
	var totalDuration time.Duration

	for _, m := range c.metrics {
		if m.Status == "success" {
			success++
		} else {
			failed++
		}
		totalDuration += m.Duration
	}

	return fmt.Sprintf("操作总数: %d, 成功: %d, 失败: %d, 总耗时: %v", total, success, failed, totalDuration)
}

// WriteToFile 将指标写入文件
func (c *Collector) WriteToFile(filePath string) error {
	data, err := c.ExportJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, []byte(data), 0o644)
}
