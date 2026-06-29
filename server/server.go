// Package server 实现了 Elasticsearch 集成的 MCP 服务器功能。
// 它为模型上下文协议（Model Context Protocol）提供 stdio 和 Streamable HTTP 协议支持。
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"

	"elasticsearch-mcp/config"
	"elasticsearch-mcp/elasticsearch"
	"elasticsearch-mcp/prompts"
	"elasticsearch-mcp/thanos"
	"elasticsearch-mcp/tools"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zuoyangs/go-mcp-common/serverauth"
)

// ElasticsearchMCPServer 表示 Elasticsearch 操作的主要 MCP 服务器
type ElasticsearchMCPServer struct {
	config          *config.Config                  // 服务器配置
	esClients       map[string]elasticsearch.Client // 多个 Elasticsearch 客户端实例
	esTools         *tools.ElasticsearchTools       // Elasticsearch 工具集合
	monitoringTools *tools.MonitoringTools          // 基于 Thanos 的监控工具集合
	mcpServer       *mcp.Server                     // 底层 MCP 服务器
	datasourceNames []string                        // 数据源名称列表
	authConfig      *serverauth.Config              // 鉴权配置
}

// NewElasticsearchMCPServer 使用提供的配置创建新的 Elasticsearch MCP 服务器实例
// 它初始化所有配置的 Elasticsearch 客户端，创建工具集合，并使用注册的所有工具设置 MCP 服务器。
// 注意：如果某些数据源连接失败，服务器仍会正常启动，但该数据源将不可用。
func NewElasticsearchMCPServer(cfg *config.Config) (*ElasticsearchMCPServer, error) {
	// 初始化多个 Elasticsearch 客户端
	esClients := make(map[string]elasticsearch.Client)
	datasourceNames := make([]string, 0, len(cfg.GetDatasources()))

	datasources := cfg.GetDatasources()
	connectedCount := 0
	failedCount := 0

	for name, dsConfig := range datasources {
		esClient, err := elasticsearch.NewClient(&dsConfig, cfg.GetElasticsearchVersion())
		if err != nil {
			// 根据配置决定是否继续：如果 skip_connection_test 为 true，则记录警告并继续
			if dsConfig.SkipConnectionTest {
				log.Printf("警告: 数据源 '%s' 连接不可用（已配置跳过连接测试）: %v", name, err)
				log.Printf("提示: 该数据源的调用将在运行时失败，但服务器可以正常启动")
				failedCount++
			} else {
				// 如果没有配置跳过连接测试，则关闭已创建的客户端并返回错误
				log.Printf("错误: 连接到数据源 '%s' 的 Elasticsearch 失败: %v", name, err)
				for _, client := range esClients {
					if closeErr := client.Close(); closeErr != nil {
						log.Printf("警告: 关闭客户端失败: %v", closeErr)
					}
				}
				return nil, fmt.Errorf("创建数据源 '%s' 的 Elasticsearch 客户端失败: %w", name, err)
			}
		} else {
			log.Printf("已连接到数据源 '%s' 的 Elasticsearch (%v)", name, dsConfig.Addresses)
			esClients[name] = esClient
			datasourceNames = append(datasourceNames, name)
			connectedCount++
		}
	}

	// 检查是否至少有一个数据源可用
	if connectedCount == 0 {
		log.Printf("警告: 没有成功连接到任何数据源，所有工具调用都将失败")
	} else {
		log.Printf("已连接到 %d 个数据源", connectedCount)
	}

	if failedCount > 0 {
		log.Printf("警告: %d 个数据源连接失败但服务器将继续运行（配置了 skip_connection_test）", failedCount)
	}

	// 使用 Elasticsearch 客户端创建工具集合
	esTools := tools.NewElasticsearchTools(esClients)

	// 创建 Thanos 监控工具集合
	thanosCfg := cfg.GetThanosConfig()
	thanosClient := thanos.NewClient(thanosCfg.Endpoint, thanosCfg.Timeout)
	monitoringTools := tools.NewMonitoringTools(thanosClient)
	log.Printf("Thanos 监控已配置: %s", thanosCfg.Endpoint)

	// 创建 MCP 服务器
	impl := &mcp.Implementation{
		Name:    cfg.Server.Name,
		Version: cfg.Server.Version,
	}
	mcpServer := mcp.NewServer(impl, &mcp.ServerOptions{
		Instructions: `Elasticsearch MCP 服务器 - 提供与多个 Elasticsearch 集群交互的工具和基于 Thanos 的 Prometheus 监控指标查询。

## ELK 日志平台架构（全局上下文）

本 Elasticsearch MCP 是 ELK 日志平台的存储检索层。完整日志链路：
k8s Pod → Fluent-Bit(采集) → Kafka(中转) → Logstash(消费处理) → Elasticsearch(存储检索)

### 日志流向
- 阿里云 ACK Pod → Fluent-Bit → aliyun-kafka → [kafka_sync] → rapid-elk-rke Kafka → Logstash → rapid-es
- 腾讯云 TKE Pod → Fluent-Bit → tencent-ckafka → [kafka_sync] → rapid-elk-rke Kafka → Logstash → rapid-es
- Hedan RKE Pod → Fluent-Bit → prod-hedan-elk-rke Kafka → Logstash → rapid-es / raven-es
- Putuo RKE Pod → Fluent-Bit → putuo-rapid-rke Kafka → [kafka_sync] → Hedan Kafka → Logstash → ES

### ES 集群
- <primary-es>: 主日志 ES，存储大部分业务日志
- <secondary-es>: 特定业务日志

### 关联 MCP 工具
- 上游: fluentbit-mcp(采集) → kafka-mcp(中转) → logstash-mcp(消费)
- 当前: elasticsearch-mcp（索引管理、日志搜索、集群监控）

### 命名规范（强制）
IDC 机房名称必须使用英文：Hedan、Putuo，严禁翻译为中文。
kafka_sync 管道描述为"跨机房 Kafka 同步"。

当用户问日志丢失/延迟问题时，需要结合上游组件一起排查。`,
	})

	// 使用 MCP 服务器注册所有 Elasticsearch 工具
	if err := registerTools(mcpServer, esTools, monitoringTools); err != nil {
		log.Printf("错误: 注册工具失败: %v", err)
		// 关闭所有客户端
		for _, client := range esClients {
			if closeErr := client.Close(); closeErr != nil {
				log.Printf("警告: 关闭客户端失败: %v", closeErr)
			}
		}
		return nil, fmt.Errorf("注册工具失败: %w", err)
	}
	log.Printf("已注册 %d 个工具（ES %d + 监控 %d + Discover %d）",
		len(esTools.GetTools())+len(tools.GetMonitoringTools())+len(esTools.GetDiscoverTools()),
		len(esTools.GetTools()), len(tools.GetMonitoringTools()), len(esTools.GetDiscoverTools()))

	// 注册 Prompts
	prompts.Register(mcpServer)
	log.Printf("已注册 Prompts")

	return &ElasticsearchMCPServer{
		config:          cfg,
		esClients:       esClients,
		esTools:         esTools,
		monitoringTools: monitoringTools,
		mcpServer:       mcpServer,
		datasourceNames: datasourceNames,
		authConfig:      &cfg.Auth,
	}, nil
}

// registerTools 使用 MCP 服务器注册所有 Elasticsearch 工具。
// 每个工具都使用其对应的处理函数注册。
func registerTools(mcpServer *mcp.Server, esTools *tools.ElasticsearchTools, monitoringTools *tools.MonitoringTools) error {
	toolsList := esTools.GetTools()
	// 追加 Discover 日志查询工具
	toolsList = append(toolsList, esTools.GetDiscoverTools()...)

	// 为每个 ES 工具创建处理函数
	for _, tool := range toolsList {
		toolName := tool.Name

		handler := func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			log.Printf("工具调用: %s", toolName)

			var args map[string]interface{}
			if len(req.Params.Arguments) > 0 {
				// 尝试直接解析 JSON
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					// 如果解析失败，尝试将参数作为字符串解析
					var argStr string
					if strErr := json.Unmarshal(req.Params.Arguments, &argStr); strErr == nil {
						if strErr := json.Unmarshal([]byte(argStr), &args); strErr != nil {
							return &mcp.CallToolResult{
								Content: []mcp.Content{
									&mcp.TextContent{Text: fmt.Sprintf("解析参数失败: %v", err)},
								},
								IsError: true,
							}, nil
						}
					} else {
						return &mcp.CallToolResult{
							Content: []mcp.Content{
								&mcp.TextContent{Text: fmt.Sprintf("解析参数失败: %v", err)},
							},
							IsError: true,
						}, nil
					}
				}
			} else {
				args = make(map[string]interface{})
			}

			result := esTools.HandleTool(ctx, toolName, args)
			if result.IsError {
				log.Printf("工具 %s 失败", toolName)
			}
			return &result, nil
		}

		mcpServer.AddTool(&tool, handler)
	}

	// 注册监控工具
	monitoringToolsList := tools.GetMonitoringTools()
	for _, tool := range monitoringToolsList {
		toolName := tool.Name

		handler := func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			log.Printf("监控工具调用: %s", toolName)

			var args map[string]interface{}
			if len(req.Params.Arguments) > 0 {
				// 尝试直接解析 JSON
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					// 如果解析失败，尝试将参数作为字符串解析
					var argStr string
					if strErr := json.Unmarshal(req.Params.Arguments, &argStr); strErr == nil {
						if strErr := json.Unmarshal([]byte(argStr), &args); strErr != nil {
							return &mcp.CallToolResult{
								Content: []mcp.Content{
									&mcp.TextContent{Text: fmt.Sprintf("解析参数失败: %v", err)},
								},
								IsError: true,
							}, nil
						}
					} else {
						return &mcp.CallToolResult{
							Content: []mcp.Content{
								&mcp.TextContent{Text: fmt.Sprintf("解析参数失败: %v", err)},
							},
							IsError: true,
						}, nil
					}
				}
			} else {
				args = make(map[string]interface{})
			}

			result := monitoringTools.HandleMonitoringTool(ctx, toolName, args)
			if result.IsError {
				log.Printf("监控工具 %s 失败", toolName)
			}
			return &result, nil
		}

		mcpServer.AddTool(&tool, handler)
	}

	return nil
}

// Start 使用配置的协议（stdio、http 或 sse）启动 MCP 服务器。
// 该方法将阻塞，直到服务器停止或遇到错误。
func (s *ElasticsearchMCPServer) Start() error {
	switch s.config.Server.Protocol {
	case "stdio":
		return s.startStdioServer()
	case "http":
		return s.startStreamableHTTP()
	case "sse":
		log.Printf("警告: SSE 协议已弃用")
		return s.startSSEServer()
	default:
		err := fmt.Errorf("不支持的协议: %s", s.config.Server.Protocol)
		log.Printf("错误: %v", err)
		return err
	}
}

// startStdioServer 使用 stdio 协议启动 MCP 服务器。
// 此模式通常用于与 LLM 工具直接集成。
func (s *ElasticsearchMCPServer) startStdioServer() error {
	log.Printf("在 stdio 上启动 MCP 服务器")

	// 创建 stdio 传输
	transport := &mcp.StdioTransport{}

	// 运行服务器
	err := s.mcpServer.Run(context.Background(), transport)
	if err != nil {
		log.Printf("错误: Stdio 服务器失败: %v", err)
	}
	return err
}

// startStreamableHTTP 使用 Streamable HTTP 协议启动 MCP 服务器。
// 此模式允许通过 HTTP 访问服务器以进行远程连接。
func (s *ElasticsearchMCPServer) startStreamableHTTP() error {
	address := fmt.Sprintf("%s:%d", s.config.Server.Address, s.config.Server.Port)

	// 创建 HTTP 处理程序
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		clientIP := getRealClientIP(r)
		log.Printf("来自 %s 的 MCP 请求", clientIP)
		return s.mcpServer
	}, &mcp.StreamableHTTPOptions{JSONResponse: true})

	// 用错误日志记录包装处理程序
	loggingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := getRealClientIP(r)

		// 认证检查：仅对 chat 交互（tools/call 等）要求认证，
		// 导入 tools（initialize、ping、tools/list、prompts/list、resources/list）无需认证
		if r.Method != http.MethodOptions && s.authConfig.Enabled {
			// 读取请求体并恢复，以便后续 handler 可以再次读取
			bodyBytes, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			// 解析 JSON-RPC method，判断是否需要认证
			needsAuth := true
			var rpcReq struct {
				Method string `json:"method"`
			}
			if json.Unmarshal(bodyBytes, &rpcReq) == nil {
				switch rpcReq.Method {
				case "initialize", "ping", "tools/list", "prompts/list", "resources/list":
					needsAuth = false
				}
			}

			if needsAuth {
				authHeader := r.Header.Get("Authorization")
				if !s.authConfig.ValidateAuth(authHeader) {
					log.Printf("AUTH FAILED | IP: %s | RPC: %s", clientIP, rpcReq.Method)
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
		}

		log.Printf("来自 %s 的 MCP 请求", clientIP)
		
		// 创建一个带注释的处理程序，记录工具调用的IP
		originalHandler := mcpHandler
		annotatedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 在请求上下文中添加客户端IP信息
			ctx := context.WithValue(r.Context(), "client_ip", clientIP)
			r = r.WithContext(ctx)
			
			originalHandler.ServeHTTP(w, r)
		})
		
		annotatedHandler.ServeHTTP(w, r)
	})

	// 启动 HTTP 服务器
	http.Handle("/mcp", loggingHandler)

	// 添加健康检查端点
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","server":"elasticsearch-mcp"}`))
	})

	log.Printf("HTTP 服务器监听在 %s", address)

	err := http.ListenAndServe(address, nil)
	if err != nil {
		log.Printf("错误: HTTP 服务器失败: %v", err)
	}
	return err
}

// getRealClientIP 从请求中获取真实的客户端IP地址
// 支持 Ingress-Nginx 跨集群场景，按优先级依次检查代理头
func getRealClientIP(r *http.Request) string {
	// 1. X-Forwarded-For: Ingress-Nginx 默认注入，取第一个（最左侧为原始客户端 IP）
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip := strings.Split(xff, ",")[0]
		return normalizeIP(strings.TrimSpace(ip))
	}

	// 2. X-Real-IP: Nginx 反向代理常用
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return normalizeIP(strings.TrimSpace(xri))
	}

	// 3. Forwarded: RFC 7239 标准头
	if fwd := r.Header.Get("Forwarded"); fwd != "" {
		if ip := parseForwardedHeader(fwd); ip != "" {
			return normalizeIP(ip)
		}
	}

	// 4. X-Original-Forwarded-For: Nginx Ingress Controller 使用
	if xoff := r.Header.Get("X-Original-Forwarded-For"); xoff != "" {
		ip := strings.Split(xoff, ",")[0]
		return normalizeIP(strings.TrimSpace(ip))
	}

	// 5. X-Envoy-External-Address: Istio / Envoy 场景
	if envoy := r.Header.Get("X-Envoy-External-Address"); envoy != "" {
		return normalizeIP(strings.TrimSpace(envoy))
	}

	// 6. 最后使用 RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return normalizeIP(ip)
}

// parseForwardedHeader 解析 RFC 7239 Forwarded 头，提取第一个 for= 的 IP
func parseForwardedHeader(fwd string) string {
	first := strings.Split(fwd, ",")[0]
	for _, part := range strings.Split(first, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "for=") {
			ip := part[4:]
			ip = strings.Trim(ip, `"`)
			ip = strings.Trim(ip, `[]`)
			if host, _, err := net.SplitHostPort(ip); err == nil {
				ip = host
			}
			return ip
		}
	}
	return ""
}

// normalizeIP 标准化 IP 地址，将 IPv6 环回和映射地址转为 IPv4
func normalizeIP(addr string) string {
	if addr == "::1" {
		return "127.0.0.1"
	}
	if ipObj := net.ParseIP(addr); ipObj != nil {
		if v4 := ipObj.To4(); v4 != nil {
			return v4.String()
		}
	}
	return addr
}

// startSSEServer 使用 SSE（服务器发送事件）协议启动 MCP 服务器。
// 警告：此协议已弃用，不建议在生产环境中使用。
func (s *ElasticsearchMCPServer) startSSEServer() error {
	address := fmt.Sprintf("%s:%d", s.config.Server.Address, s.config.Server.Port)

	// 创建 SSE 处理程序
	sseHandler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
		clientIP := getRealClientIP(r)
		log.Printf("来自 %s 的 SSE 请求", clientIP)
		return s.mcpServer
	}, nil)

	// 使用 SSE 端点启动 HTTP 服务器
	http.Handle("/sse", sseHandler)

	log.Printf("SSE 服务器监听在 %s", address)

	err := http.ListenAndServe(address, nil)
	if err != nil {
		log.Printf("错误: SSE 服务器失败: %v", err)
	}
	return err
}

// Stop 优雅地关闭 MCP 服务器并清理资源。
func (s *ElasticsearchMCPServer) Stop() error {
	// 关闭所有 Elasticsearch 客户端连接
	var lastErr error
	for name, client := range s.esClients {
		if client != nil {
			if err := client.Close(); err != nil {
				log.Printf("错误: 关闭数据源 '%s' 的 Elasticsearch 客户端失败: %v", name, err)
				lastErr = err
			}
		}
	}

	return lastErr
}

// GetInfo 返回有关服务器配置和状态的信息。
// 这对于调试和监控目的很有用。
func (s *ElasticsearchMCPServer) GetInfo() map[string]interface{} {
	// 收集所有数据源信息
	datasources := make(map[string]interface{})
	for name, dsConfig := range s.config.GetDatasources() {
		datasources[name] = map[string]interface{}{
			"addresses": dsConfig.Addresses,
			"connected": s.esClients[name] != nil,
		}
	}

	return map[string]interface{}{
		"name":                  s.config.Server.Name,
		"version":               s.config.Server.Version,
		"protocol":              s.config.Server.Protocol,
		"default_datasource":    s.config.DefaultDatasource,
		"datasources":           datasources,
		"total_datasources":     len(s.esClients),
		"elasticsearch_version": s.config.GetElasticsearchVersion(),
	}
}