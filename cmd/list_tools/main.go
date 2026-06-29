package main

import (
	"fmt"

	"elasticsearch-mcp/tools"
)

func main() {
	// 用空 clients 创建工具集，只为获取工具定义
	et := tools.NewElasticsearchTools(nil)

	fmt.Println("===== ES 原生 API 工具 =====")
	for i, t := range et.GetTools() {
		fmt.Printf("  %2d. %-35s %s\n", i+1, t.Name, firstLine(t.Description))
	}

	fmt.Println()
	fmt.Println("===== Discover 日志查询工具 =====")
	for i, t := range et.GetDiscoverTools() {
		fmt.Printf("  %2d. %-35s %s\n", i+1, t.Name, firstLine(t.Description))
	}

	fmt.Println()
	fmt.Println("===== Prometheus 监控工具 =====")
	for i, t := range tools.GetMonitoringTools() {
		fmt.Printf("  %2d. %-45s %s\n", i+1, t.Name, firstLine(t.Description))
	}

	total := len(et.GetTools()) + len(et.GetDiscoverTools()) + len(tools.GetMonitoringTools())
	fmt.Printf("\n共计 %d 个工具（ES %d + Discover %d + 监控 %d）\n",
		total, len(et.GetTools()), len(et.GetDiscoverTools()), len(tools.GetMonitoringTools()))
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}
