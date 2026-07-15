package backup

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

// DriverMetadata describes a backup driver
type DriverMetadata struct {
	Name                 string
	Version              string
	Description          string
	SupportedActions     []string
	SupportedBackupTypes []BackupType
}

// DriverFactory creates a new backup driver instance
type DriverFactory func(config *DBConfig) (DatabaseBackup, error)

// driverInfo 注册表中存储的驱动信息
type driverInfo struct {
	metadata DriverMetadata
	factory  DriverFactory
}

// driverRegistry 驱动注册表
var driverRegistry = struct {
	sync.RWMutex
	drivers map[string]driverInfo
}{
	drivers: make(map[string]driverInfo),
}

// RegisterDriver 注册数据库备份驱动
func RegisterDriver(metadata DriverMetadata, factory DriverFactory) error {
	if factory == nil {
		return errors.New("backup: RegisterDriver called with nil factory")
	}
	if metadata.Name == "" {
		return errors.New("backup: RegisterDriver called with empty driver name")
	}

	driverRegistry.Lock()
	defer driverRegistry.Unlock()

	if _, exists := driverRegistry.drivers[metadata.Name]; exists {
		return errors.New("backup: RegisterDriver called twice for driver " + metadata.Name)
	}
	driverRegistry.drivers[metadata.Name] = driverInfo{
		metadata: metadata,
		factory:  factory,
	}
	return nil
}

// UnregisterDriver 注销数据库备份驱动
func UnregisterDriver(name string) {
	driverRegistry.Lock()
	defer driverRegistry.Unlock()
	delete(driverRegistry.drivers, name)
}

// GetDriverMetadata 获取指定驱动的元数据
func GetDriverMetadata(name string) (DriverMetadata, bool) {
	driverRegistry.RLock()
	defer driverRegistry.RUnlock()
	info, exists := driverRegistry.drivers[name]
	return info.metadata, exists
}

// getDriverFactory 获取指定驱动的工厂函数
func getDriverFactory(name string) (DriverFactory, bool) {
	driverRegistry.RLock()
	defer driverRegistry.RUnlock()
	info, exists := driverRegistry.drivers[name]
	return info.factory, exists
}

// ListDrivers 返回所有已注册的驱动名称列表
func ListDrivers() []string {
	driverRegistry.RLock()
	defer driverRegistry.RUnlock()
	names := make([]string, 0, len(driverRegistry.drivers))
	for name := range driverRegistry.drivers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListDriverMetadata 返回所有已注册驱动的元数据列表
func ListDriverMetadata() []DriverMetadata {
	driverRegistry.RLock()
	defer driverRegistry.RUnlock()
	metadataList := make([]DriverMetadata, 0, len(driverRegistry.drivers))
	for _, info := range driverRegistry.drivers {
		metadataList = append(metadataList, info.metadata)
	}
	sort.Slice(metadataList, func(i, j int) bool {
		return metadataList[i].Name < metadataList[j].Name
	})
	return metadataList
}

// NewBackup 创建数据库备份实例
func NewBackup(config *DBConfig) (DatabaseBackup, error) {
	if config == nil {
		return nil, errors.New("config 不能为空")
	}
	if config.Type == "" {
		return nil, errors.New("必须指定数据库类型")
	}

	factory, exists := getDriverFactory(config.Type)
	if !exists {
		return nil, errors.New("不支持的数据库类型: " + config.Type + ", 支持的类型: " + formatDriverList())
	}

	return factory(config)
}

// ValidateDriverConfig 验证驱动配置（不创建实例）
func ValidateDriverConfig(config *DBConfig) error {
	if config == nil {
		return errors.New("config 不能为空")
	}
	if config.Type == "" {
		return errors.New("必须指定数据库类型")
	}

	if _, exists := getDriverFactory(config.Type); !exists {
		return errors.New("不支持的数据库类型: " + config.Type + ", 支持的类型: " + formatDriverList())
	}

	return nil
}

func formatDriverList() string {
	drivers := ListDrivers()
	if len(drivers) == 0 {
		return "无"
	}
	return strings.Join(drivers, ", ")
}
