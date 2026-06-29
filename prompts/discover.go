package prompts

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ===== 日志排查助手 =====

var promptLogInvestigation = &mcp.Prompt{
	Name:        "es_log_investigation",
	Description: "Elasticsearch 日志排查助手：引导式日志分析，从聚合摘要到精确定位，帮助开发者快速排查问题。",
	Arguments: []*mcp.PromptArgument{
		{Name: "datasource", Description: "ES 数据源名称", Required: true},
		{Name: "index", Description: "索引名称或模式（如 app-logs-*）", Required: true},
		{Name: "problem", Description: "问题描述（如：用户下单失败、接口超时、服务 500 错误）", Required: true},
		{Name: "start_time", Description: "开始时间（如 now-1h、now-30m）", Required: false},
		{Name: "end_time", Description: "结束时间（默认 now）", Required: false},
	},
}

func handleLogInvestigation(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	ds := req.Params.Arguments["datasource"]
	idx := req.Params.Arguments["index"]
	problem := req.Params.Arguments["problem"]
	startTime := req.Params.Arguments["start_time"]
	endTime := req.Params.Arguments["end_time"]

	if startTime == "" {
		startTime = "now-1h"
	}
	if endTime == "" {
		endTime = "now"
	}

	instruction := fmt.Sprintf(`请帮我排查以下问题：%s

目标索引：%s（数据源：%s）
时间范围：%s ~ %s

## 排查步骤（严格按顺序执行）

**第一步：全貌摘要**
调用 es_log_summary（datasource="%s", index="%s", start_time="%s", end_time="%s"）
→ 了解日志总量、级别分布、错误消息 Top N

**第二步：根据摘要结果，针对性查询**
- 如果 ERROR 日志较多，调用 es_discover 搜索 ERROR 日志：
  es_discover（datasource="%s", index="%s", keyword="level:ERROR", start_time="%s", end_time="%s", size=20）
- 如果问题描述中有关键词，直接搜索：
  es_discover（datasource="%s", index="%s", keyword="<从问题描述提取的关键词>", start_time="%s", end_time="%s"）

**第三步：查看关键错误的上下文**
找到可疑的错误日志后，调用 es_log_context 查看前后日志：
es_log_context（datasource="%s", index="%s", timestamp="<错误日志的时间戳>"）

**第四步：如果需要了解索引结构**
调用 es_field_stats（datasource="%s", index="%s"）了解有哪些字段可以用于过滤

## 输出要求
1. 先给出问题概览（日志量、错误分布）
2. 列出关键错误日志（时间、级别、消息摘要）
3. 分析可能的根因
4. 给出排查建议或下一步操作`,
		problem, idx, ds, startTime, endTime,
		ds, idx, startTime, endTime,
		ds, idx, startTime, endTime,
		ds, idx, startTime, endTime,
		ds, idx,
		ds, idx)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("日志排查：%s", problem),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}
