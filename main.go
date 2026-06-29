package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"elasticsearch-mcp/config"
	"elasticsearch-mcp/server"

	"github.com/zuoyangs/go-mcp-common/serverauth"
)

func main() {
	// 格式: 2006/01/02 15:04:05 [INFO] 消息内容
	log.SetFlags(log.Ldate | log.Ltime)

	log.Printf("正在启动 elasticsearch-mcp MCP 服务器...")

	// 从环境变量加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("FATAL: 加载配置失败: %v", err)
	}

	// 记录必要的启动信息
	log.Printf("协议: %s", cfg.Server.Protocol)
	if cfg.Server.Protocol == "http" || cfg.Server.Protocol == "sse" {
		log.Printf("监听地址: %s:%d", cfg.Server.Address, cfg.Server.Port)
	}

	// 显示数据源信息
	datasources := cfg.GetDatasources()
	log.Printf("Elasticsearch 数据源数量: %d", len(datasources))
	log.Printf("Elasticsearch 默认数据源: %s", cfg.DefaultDatasource)

	for name, ds := range datasources {
		sslStatus := "[SSL]"
		if !ds.SSL {
			sslStatus = "[NO-SSL]"
		}
		log.Printf(" %s 数据源 '%s': %v", sslStatus, name, ds.Addresses)
	}

	// 记录鉴权信息
	if cfg.Auth.Enabled {
		log.Printf("鉴权: 已启用 | 用户数: %d", len(cfg.Auth.Users))
		if cfg.Auth.Token != "" {
			log.Printf("全局 Bearer Token: %s", serverauth.MaskToken(cfg.Auth.Token))
		}
	} else {
		log.Printf("鉴权: 已禁用，所有请求均允许")
	}

	// 创建 Elasticsearch MCP 服务器实例
	mcpServer, err := server.NewElasticsearchMCPServer(cfg)
	if err != nil {
		log.Fatalf("FATAL: 创建 elasticsearch-mcp MCP 服务器失败: %v", err)
	}

	// 设置信号处理以实现优雅关闭
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 在单独的 goroutine 中启动服务器
	serverErrChan := make(chan error, 1)
	go func() {
		defer close(serverErrChan)
		if err := mcpServer.Start(); err != nil {
			log.Printf("错误: 服务器失败: %v", err)
			serverErrChan <- err
		}
	}()

	// 给服务器一点时间启动
	time.Sleep(1 * time.Second)

	if cfg.Server.Protocol == "http" || cfg.Server.Protocol == "sse" {
		endpoint := "mcp"
		if cfg.Server.Protocol == "sse" {
			endpoint = "sse"
		}
		log.Printf("elasticsearch-mcp MCP 服务器运行在: http://%s:%d/%s", cfg.Server.Address, cfg.Server.Port, endpoint)
	}

	// 等待信号或服务器错误
	select {
	case sig := <-sigChan:
		log.Printf("收到信号: %v, 正在关闭...", sig)
	case err := <-serverErrChan:
		if err != nil {
			log.Printf("服务器错误: %v", err)
		}
	}

	// 优雅地关闭服务器
	if err := mcpServer.Stop(); err != nil {
		log.Printf("关闭过程中出现错误: %v", err)
	}

	log.Printf("服务器已停止")
}
