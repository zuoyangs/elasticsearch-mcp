// Package config 为 Elasticsearch MCP 服务器提供配置管理功能。
// 它支持从环境变量加载配置，并提供合理的默认值。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zuoyangs/go-mcp-common/serverauth"
	"gopkg.in/yaml.v3"
)

// RancherConfig 单个 Rancher 机房的配置
type RancherConfig struct {
	Endpoint      string          `yaml:"endpoint"`
	Token         string          `yaml:"REDACTED_TOKEN"`
	SkipTLSVerify bool            `yaml:"skip_tls_verify"`
	Clusters      []ClusterConfig `yaml:"clusters"`
}

// ClusterConfig 某个 Rancher 下的 k8s 集群配置
type ClusterConfig struct {
	Name       string   `yaml:"name"`
	ClusterID  string   `yaml:"cluster_id"`
	Namespaces []string `yaml:"namespaces"`
}

// ThanosConfig 包含 Thanos Query 连接配置
type ThanosConfig struct {
	// Endpoint Thanos Query 的 HTTP 地址，如 https://thanos-query.example.com
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint"`

	// Timeout 请求超时时间
	Timeout time.Duration `mapstructure:"timeout" yaml:"timeout"`
}

// Config 包含 Elasticsearch MCP 服务器的完整配置
type Config struct {
	// Ranchers 多 Rancher 机房配置
	Ranchers map[string]RancherConfig `mapstructure:"ranchers"`

	// Elasticsearch 连接和客户端配置（向后兼容）
	Elasticsearch ElasticsearchConfig `mapstructure:"elasticsearch"`

	// Datasources 支持多个 Elasticsearch 数据源配置
	// key 为数据源唯一标识符，value 为对应的配置
	Datasources map[string]ElasticsearchConfig `mapstructure:"datasources"`

	// DefaultDatasource 指定默认数据源的名称
	DefaultDatasource string `mapstructure:"default_datasource"`

	// ElasticsearchVersion 指定 Elasticsearch 版本（用于客户端兼容性）
	ElasticsearchVersion string `mapstructure:"elasticsearch_version"`

	// MCP 服务器配置
	Server ServerConfig `mapstructure:"server"`

	// Auth 鉴权配置
	Auth serverauth.Config `mapstructure:"auth"`

	// Thanos Thanos Query 配置（用于 Prometheus 指标查询）
	Thanos ThanosConfig `mapstructure:"thanos"`
}

// FileConfig 表示配置文件中的结构
type FileConfig struct {
	Ranchers             map[string]RancherConfig       `yaml:"ranchers,omitempty"`
	Elasticsearch        ElasticsearchConfig            `yaml:"elasticsearch"`
	Datasources          map[string]ElasticsearchConfig `yaml:"datasources,omitempty"`
	DefaultDatasource    string                         `yaml:"default_datasource,omitempty"`
	ElasticsearchVersion string                         `yaml:"elasticsearch_version,omitempty"`
	Server               ServerConfig                   `yaml:"server"`
	Auth                 serverauth.Config              `yaml:"auth"`
	Thanos               ThanosConfig                   `yaml:"thanos,omitempty"`
}

// InfrastructureConfig 描述 ES 数据源所在的基础设施位置，
// 将 datasource 与 Rancher 机房、k8s 集群、namespace 以及 Thanos 监控标签绑定。
// 同一个 namespace 下只有一个 ES 集群，因此 cluster + namespace 即可唯一定位。
// 这样工具可以自动完成：
//   - datasource → rancher 机房 → k8s 集群 → namespace → pod 列表（查节点名称）
//   - datasource → thanos cluster + namespace → 监控指标（免手动传参）
type InfrastructureConfig struct {
	// Rancher 对应 ranchers 配置中的 key（机房名），如 "hedan"、"putuo"
	Rancher string `yaml:"rancher" mapstructure:"rancher"`

	// ClusterID Rancher 中的 k8s 集群 ID，如 "c-j8fxh"
	ClusterID string `yaml:"cluster_id" mapstructure:"cluster_id"`

	// Namespace ES StatefulSet 所在的 k8s namespace，如 "elastic-system"
	Namespace string `yaml:"namespace" mapstructure:"namespace"`

	// ThanosCluster Thanos/Prometheus 中的 cluster 标签值，如 "hedan-elk-rke"
	ThanosCluster string `yaml:"thanos_cluster" mapstructure:"thanos_cluster"`

	// WorkloadPrefix ES StatefulSet 名称前缀，用于匹配 pod 名，如 "logging-elastic-hot"
	WorkloadPrefix string `yaml:"workload_prefix" mapstructure:"workload_prefix"`
}

// ElasticsearchConfig 包含所有 Elasticsearch 连接设置
type ElasticsearchConfig struct {
	// Addresses 是 Elasticsearch 集群地址列表
	Addresses []string `yaml:"addresses" mapstructure:"addresses"`

	// Username 用于基本认证
	Username string `yaml:"REDACTED_USERNAME" mapstructure:"REDACTED_USERNAME"`

	// Password 用于基本认证
	Password string `yaml:"REDACTED_PASSWORD" mapstructure:"REDACTED_PASSWORD"`

	// SSL 启用 SSL/TLS 连接
	SSL bool `yaml:"ssl" mapstructure:"ssl"`

	// InsecureSkipVerify 绕过证书验证（仅用于开发）
	InsecureSkipVerify bool `yaml:"insecure_skip_verify" mapstructure:"insecure_skip_verify"`

	// Timeout Elasticsearch HTTP 请求的超时时间（字符串格式，如 "30s"）
	Timeout string `yaml:"timeout" mapstructure:"timeout"`

	// MaxRetries 指定失败请求的最大重试次数
	MaxRetries int `yaml:"max_retries" mapstructure:"max_retries"`

	// SkipConnectionTest 跳过启动时的连接测试，允许在 Elasticsearch 不可用时启动
	SkipConnectionTest bool `yaml:"skip_connection_test" mapstructure:"skip_connection_test"`

	// Infrastructure 基础设施关联配置（可选），绑定 Rancher/k8s/Thanos 信息
	Infrastructure *InfrastructureConfig `yaml:"infrastructure,omitempty" mapstructure:"infrastructure"`
}

// ServerConfig 包含 MCP 服务器设置
type ServerConfig struct {
	// Name MCP 服务器名称
	Name string `yaml:"name" mapstructure:"name"`

	// Version MCP 服务器版本
	Version string `yaml:"version" mapstructure:"version"`

	// Protocol 指定通信协议（stdio、http 或 sse）
	// 注意：SSE 协议已弃用，不建议在生产环境中使用
	Protocol string `yaml:"protocol" mapstructure:"protocol"`

	// Address HTTP 服务器地址（仅在协议为 http 时使用）
	Address string `yaml:"address" mapstructure:"address"`

	// Port HTTP 服务器端口（仅在协议为 http 时使用）
	Port int `yaml:"port" mapstructure:"port"`
}

// LoadConfig 从配置文件加载配置，如果配置文件不存在则使用环境变量
func LoadConfig() (*Config, error) {
	config, err := loadConfigFromFile()
	if err != nil {
		// 如果文件加载失败，回退到环境变量模式
		return loadConfigFromEnv()
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	return config, nil
}

// loadConfigFromFile 从配置文件加载配置
func loadConfigFromFile() (*Config, error) {
	// 尝试多个可能的配置文件路径
	configPaths := []string{
		"etc/elasticsearch.yaml",
		"etc/config.yaml",
		"elasticsearch.yaml",
		"config.yaml",
	}

	var configData []byte
	var configPath string

	for _, path := range configPaths {
		if data, err := os.ReadFile(path); err == nil {
			configData = data
			configPath = path
			break
		}
	}

	if configData == nil {
		return nil, fmt.Errorf("未找到配置文件")
	}

	var fileConfig FileConfig
	if err := yaml.Unmarshal(configData, &fileConfig); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", configPath, err)
	}

	config := &Config{
		Ranchers:             fileConfig.Ranchers,
		Elasticsearch:        fileConfig.Elasticsearch,
		Datasources:          fileConfig.Datasources,
		DefaultDatasource:    fileConfig.DefaultDatasource,
		ElasticsearchVersion: fileConfig.ElasticsearchVersion,
		Server:               fileConfig.Server,
		Auth:                 fileConfig.Auth,
		Thanos:               fileConfig.Thanos,
	}

	// 如果没有配置多数据源，则使用默认配置作为单一数据源
	if len(config.Datasources) == 0 {
		config.Datasources = map[string]ElasticsearchConfig{
			"default": fileConfig.Elasticsearch,
		}
	}

	// 如果未指定默认数据源，使用第一个数据源作为默认
	if config.DefaultDatasource == "" {
		for name := range config.Datasources {
			config.DefaultDatasource = name
			break
		}
	}

	return config, nil
}

// loadConfigFromEnv 从环境变量加载配置（向后兼容）
func loadConfigFromEnv() (*Config, error) {
	// 加载默认数据源配置（向后兼容）
	defaultConfig := ElasticsearchConfig{
		Addresses:          getEnvStringSlice("ES_ADDRESSES", []string{"http://127.0.0.1:9200"}),
		Username:           getEnvString("ES_USERNAME", ""),
		Password:           getEnvString("ES_PASSWORD", ""),
		SSL:                getEnvBool("ES_SSL", false),
		InsecureSkipVerify: getEnvBool("ES_INSECURE_SKIP_VERIFY", false),
		Timeout:            getEnvString("ES_TIMEOUT", "30s"),
		MaxRetries:         getEnvInt("ES_MAX_RETRIES", 3),
		SkipConnectionTest: getEnvBool("ES_SKIP_CONNECTION_TEST", false),
	}

	config := &Config{
		Elasticsearch: defaultConfig,
		Datasources:   make(map[string]ElasticsearchConfig),
		Server: ServerConfig{
			Name:     getEnvString("MCP_SERVER_NAME", "Elasticsearch MCP Server"),
			Version:  getEnvString("MCP_SERVER_VERSION", "1.0.0"),
			Protocol: getEnvString("MCP_PROTOCOL", "stdio"),
			Address:  getEnvString("MCP_ADDRESS", "localhost"),
			Port:     getEnvInt("MCP_PORT", 8080),
		},
	}

	// 加载多数据源配置
	if err := config.loadDatasources(); err != nil {
		return nil, fmt.Errorf("加载数据源配置失败: %w", err)
	}

	// 设置默认数据源
	config.DefaultDatasource = getEnvString("ES_DEFAULT_DATASOURCE", "default")

	// 如果没有配置多数据源，则使用默认配置作为单一数据源
	if len(config.Datasources) == 0 {
		config.Datasources["default"] = defaultConfig
	}

	return config, nil
}

// loadDatasources 从环境变量加载多数据源配置
func (c *Config) loadDatasources() error {
	// 支持两种配置方式：
	// 1. ES_DATASOURCES_JSON: JSON 格式的完整数据源配置
	// 2. 多个以 ES_DATASOURCE_ 开头的环境变量

	// 方式1: JSON 配置
	if jsonConfig := getEnvString("ES_DATASOURCES_JSON", ""); jsonConfig != "" {
		if err := json.Unmarshal([]byte(jsonConfig), &c.Datasources); err != nil {
			return fmt.Errorf("解析 ES_DATASOURCES_JSON 失败: %w", err)
		}
		return nil
	}

	// 方式2: 环境变量配置
	// 格式: ES_DATASOURCE_{NAME}_{FIELD}={VALUE}
	// 例如: ES_DATASOURCE_PROD_ADDRESSES=http://prod-es:9200
	//       ES_DATASOURCE_DEV_ADDRESSES=http://dev-es:9200

	datasourceEnvVars := getEnvVarsWithPrefix("ES_DATASOURCE_")
	if len(datasourceEnvVars) == 0 {
		return nil // 没有多数据源配置，使用默认配置
	}

	// 解析数据源配置
	datasourceMap := make(map[string]map[string]string)
	for key, value := range datasourceEnvVars {
		// 移除前缀
		trimmedKey := strings.TrimPrefix(key, "ES_DATASOURCE_")

		// 分割数据源名称和字段名
		parts := strings.SplitN(trimmedKey, "_", 2)
		if len(parts) != 2 {
			continue // 无效格式，跳过
		}

		datasourceName := strings.ToLower(parts[0])
		fieldName := parts[1]

		if datasourceMap[datasourceName] == nil {
			datasourceMap[datasourceName] = make(map[string]string)
		}
		datasourceMap[datasourceName][fieldName] = value
	}

	// 转换为 ElasticsearchConfig
	for name, fields := range datasourceMap {
		config := ElasticsearchConfig{}

		// 解析各个字段
		if addresses, exists := fields["ADDRESSES"]; exists {
			config.Addresses = strings.Split(addresses, ",")
		} else {
			config.Addresses = []string{"http://127.0.0.1:9200"} // 默认值
		}

		config.Username = fields["USERNAME"]
		config.Password = fields["PASSWORD"]

		if ssl, exists := fields["SSL"]; exists {
			if parsed, err := strconv.ParseBool(ssl); err == nil {
				config.SSL = parsed
			}
		}

		if insecure, exists := fields["INSECURE_SKIP_VERIFY"]; exists {
			if parsed, err := strconv.ParseBool(insecure); err == nil {
				config.InsecureSkipVerify = parsed
			}
		}

		if timeout, exists := fields["TIMEOUT"]; exists {
			config.Timeout = timeout
		} else {
			config.Timeout = "30s"
		}

		if retries, exists := fields["MAX_RETRIES"]; exists {
			if parsed, err := strconv.Atoi(retries); err == nil {
				config.MaxRetries = parsed
			} else {
				config.MaxRetries = 3
			}
		} else {
			config.MaxRetries = 3
		}

		if skipTest, exists := fields["SKIP_CONNECTION_TEST"]; exists {
			if parsed, err := strconv.ParseBool(skipTest); err == nil {
				config.SkipConnectionTest = parsed
			}
		}

		c.Datasources[name] = config
	}

	return nil
}

// Validate 检查配置是否有效，如果无效则返回错误
func (c *Config) Validate() error {
	// 验证数据源配置
	if len(c.Datasources) == 0 {
		return fmt.Errorf("必须至少配置一个数据源")
	}

	// 验证每个数据源
	for name, ds := range c.Datasources {
		if len(ds.Addresses) == 0 {
			return fmt.Errorf("数据源 '%s' 必须指定至少一个 Elasticsearch 地址", name)
		}
	}

	// 验证默认数据源
	if c.DefaultDatasource != "" {
		if _, exists := c.Datasources[c.DefaultDatasource]; !exists {
			return fmt.Errorf("默认数据源 '%s' 不存在于配置的数据源中", c.DefaultDatasource)
		}
	} else {
		// 如果未指定默认数据源，使用第一个数据源作为默认
		for name := range c.Datasources {
			c.DefaultDatasource = name
			break
		}
	}

	// 验证服务器配置
	if c.Server.Protocol != "stdio" && c.Server.Protocol != "http" && c.Server.Protocol != "sse" {
		return fmt.Errorf("不支持的协议: %s, 支持的协议: stdio, http, sse (已弃用)", c.Server.Protocol)
	}

	if (c.Server.Protocol == "http" || c.Server.Protocol == "sse") && c.Server.Port <= 0 {
		return fmt.Errorf("HTTP/SSE 协议需要有效的端口号")
	}

	return nil
}

// GetElasticsearchVersion 返回 Elasticsearch 版本，按优先级顺序：配置文件 > 环境变量 > 默认值
func (c *Config) GetElasticsearchVersion() string {
	// 首先检查配置文件中的版本
	if c.ElasticsearchVersion != "" {
		return c.ElasticsearchVersion
	}
	// 然后检查环境变量
	if envVersion := getEnvString("ES_VERSION", ""); envVersion != "" {
		return envVersion
	}
	// 最后使用默认值
	return "7"
}

// GetDatasources 返回所有配置的数据源
func (c *Config) GetDatasources() map[string]ElasticsearchConfig {
	return c.Datasources
}

// GetDefaultDatasource 返回默认数据源配置
func (c *Config) GetDefaultDatasource() (string, ElasticsearchConfig) {
	if c.DefaultDatasource == "" {
		// 如果未设置默认数据源，返回第一个
		for name, config := range c.Datasources {
			return name, config
		}
	}
	return c.DefaultDatasource, c.Datasources[c.DefaultDatasource]
}

// GetDatasource 返回指定名称的数据源配置
func (c *Config) GetDatasource(name string) (ElasticsearchConfig, bool) {
	config, exists := c.Datasources[name]
	return config, exists
}

// GetThanosConfig 返回 Thanos 配置，按优先级：配置文件 > 环境变量 > 默认值
func (c *Config) GetThanosConfig() ThanosConfig {
	cfg := c.Thanos
	if cfg.Endpoint == "" {
		cfg.Endpoint = getEnvString("THANOS_ENDPOINT", "https://thanos-query.<your-domain>.com")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = getEnvDuration("THANOS_TIMEOUT", 30*time.Second)
	}
	return cfg
}

// 环境变量辅助函数

// getEnvString 返回环境变量值或默认值（如果未设置）
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvStringSlice 将逗号分隔的环境变量作为切片返回
func getEnvStringSlice(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return strings.Split(value, ",")
}

// getEnvInt 将环境变量值作为整数返回或返回默认值
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}

	return intValue
}

// getEnvBool 将环境变量值作为布尔值返回或返回默认值
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}

	return boolValue
}

// getEnvDuration 将环境变量值作为持续时间返回或返回默认值
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}

	return duration
}

// getEnvVarsWithPrefix 返回所有以指定前缀开头的环境变量
func getEnvVarsWithPrefix(prefix string) map[string]string {
	result := make(map[string]string)

	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		if len(pair) == 2 && strings.HasPrefix(pair[0], prefix) {
			result[pair[0]] = pair[1]
		}
	}

	return result
}
