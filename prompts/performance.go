package prompts

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ===== 3. 性能问题排查 =====

var promptPerformanceTroubleshoot = &mcp.Prompt{
	Name:        "es_performance_troubleshoot",
	Description: "Elasticsearch 搜索/写入性能问题排查：定位慢查询索引、写入热点、线程池拒绝、GC 瓶颈、缓存命中率等性能问题的根因。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
		{Name: "datasource", Description: "ES 数据源名称", Required: true},
		{Name: "symptom", Description: "症状描述，如：搜索慢、写入慢、429错误、超时", Required: true},
	},
}

func handlePerformanceTroubleshoot(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]
	ds := req.Params.Arguments["datasource"]
	symptom := req.Params.Arguments["symptom"]

	instruction := fmt.Sprintf(`ES 集群 %s/%s 出现性能问题，症状：%s。

**第一步** es_monitor_index_perf（cluster="%s", namespace="%s"）— 索引级搜索/写入 OPS 和延迟
**第二步** es_monitor_thread_pool（cluster="%s", namespace="%s"）— search/write/bulk rejected
**第三步** es_monitor_nodes_latency（cluster="%s", namespace="%s"）— 定位延迟热点节点
**第四步** es_nodes_hot_threads（datasource="%s"）— CPU 热线程
**第五步** es_monitor_nodes_gc（cluster="%s", namespace="%s"）— GC 频率和耗时
**第六步** es_monitor_index_cache（cluster="%s", namespace="%s"）— 缓存命中率
**第七步** es_monitor_segments（cluster="%s", namespace="%s"）— 段数量

输出：瓶颈定位（CPU/GC/磁盘IO/线程池）、问题索引或节点、优化建议（标注风险等级）。`,
		c, ns, symptom, c, ns, c, ns, c, ns, ds, c, ns, c, ns, c, ns)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES %s/%s 性能排查 - %s", c, ns, symptom),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 8. 慢查询分析 =====

var promptSlowQueryAnalysis = &mcp.Prompt{
	Name:        "es_slow_query_analysis",
	Description: "Elasticsearch 慢查询分析：从索引级搜索延迟、缓存命中率、段数量、节点负载等多维度分析搜索性能问题。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
		{Name: "datasource", Description: "ES 数据源名称", Required: true},
		{Name: "index", Description: "目标索引名称（可选）", Required: false},
	},
}

func handleSlowQueryAnalysis(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]
	ds := req.Params.Arguments["datasource"]
	idx := req.Params.Arguments["index"]

	indexStep := ""
	if idx != "" {
		indexStep = fmt.Sprintf(`
**额外** es_get_mapping（datasource="%s", index="%s"）+ es_get_settings + es_index_stats`, ds, idx)
	}

	instruction := fmt.Sprintf(`请分析 ES 集群 %s/%s 的搜索性能问题。

**第一步** es_monitor_index_perf（cluster="%s", namespace="%s"）— 找出延迟最高的索引
**第二步** es_monitor_index_cache（cluster="%s", namespace="%s"）— Cache 命中率 <50%%%% 说明缓存效果差
**第三步** es_monitor_segments（cluster="%s", namespace="%s"）— 段数 >50/索引 影响搜索性能
**第四步** es_monitor_nodes_cpu_memory（cluster="%s", namespace="%s"）— 节点 CPU 过高%s

输出：搜索延迟 Top 10、缓存命中率、段数异常索引、瓶颈定位、优化建议。`, c, ns, c, ns, c, ns, c, ns, c, ns, indexStep)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES %s/%s 慢查询分析", c, ns),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 9. 日志搜索助手 =====

var promptLogSearch = &mcp.Prompt{
	Name:        "es_log_search",
	Description: "Elasticsearch 日志搜索助手：根据自然语言描述自动构建 DSL 查询，搜索指定服务/时间范围/关键词的日志。",
	Arguments: []*mcp.PromptArgument{
		{Name: "datasource", Description: "ES 数据源名称", Required: true},
		{Name: "description", Description: "搜索需求描述，如：查找 order-service 最近 1 小时的 ERROR 日志", Required: true},
	},
}

func handleLogSearch(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	ds := req.Params.Arguments["datasource"]
	desc := req.Params.Arguments["description"]

	instruction := fmt.Sprintf(`请在 ES 数据源 "%s" 中搜索日志：%s

**第一步** 确定索引 — 如不确定先调 es_list_indices（datasource="%s"）
**第二步** 构建 DSL 查询 — 调 es_search（datasource="%s"），关键词用 match，时间用 range @timestamp，默认按 @timestamp desc，返回 20 条
**第三步** 以易读格式展示结果`, ds, desc, ds, ds)

	return &mcp.GetPromptResult{
		Description: "日志搜索",
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 14. ES 性能深度分析 =====

var promptESPerformanceDeepDive = &mcp.Prompt{
	Name:        "es_performance_deep_dive",
	Description: "Elasticsearch 性能深度分析：从搜索延迟、写入吞吐、线程池饱和度、JVM/GC、缓存命中率、节点均衡性等多维度全面评估，输出性能评分卡和调优建议。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
		{Name: "datasource", Description: "ES 数据源名称（可选）", Required: false},
	},
}

func handleESPerformanceDeepDive(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]
	ds := req.Params.Arguments["datasource"]

	hotThreadStep := ""
	if ds != "" {
		hotThreadStep = fmt.Sprintf(`
**第七步** es_nodes_hot_threads（datasource="%s"）— CPU 热线程堆栈`, ds)
	}

	instruction := fmt.Sprintf(`请对 ES 集群 %s/%s 进行性能深度分析。

**第一步** es_monitor_search_performance（cluster="%s", namespace="%s"）
**第二步** es_monitor_write_performance（cluster="%s", namespace="%s"）
**第三步** es_monitor_thread_pool（cluster="%s", namespace="%s"）
**第四步** es_monitor_nodes_gc + es_monitor_nodes_jvm_heap（cluster="%s", namespace="%s"）
**第五步** es_monitor_node_balance（cluster="%s", namespace="%s"）
**第六步** es_monitor_breakers（cluster="%s", namespace="%s"）%s

输出性能评分卡（搜索延迟/Cache命中率/rejected/写入延迟/JVM堆/GC/均衡/断路器）+ 瓶颈定位 + 调优建议。`,
		c, ns, c, ns, c, ns, c, ns, c, ns, c, ns, c, ns, hotThreadStep)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES %s/%s 性能深度分析", c, ns),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 16. ES 写入性能专项分析 =====

var promptESWritePerformanceAnalysis = &mcp.Prompt{
	Name:        "es_write_performance_analysis",
	Description: "Elasticsearch 写入性能专项分析：从 Indexing 吞吐、Bulk 拒绝、Translog、Refresh/Flush/Merge 延迟、磁盘 I/O、GC 暂停等维度全面分析写入瓶颈。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
		{Name: "datasource", Description: "ES 数据源名称（可选）", Required: false},
	},
}

func handleESWritePerformanceAnalysis(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]

	instruction := fmt.Sprintf(`请对 ES 集群 %s/%s 进行写入性能专项分析。

阈值参考：Indexing延迟 <20ms 健康 / Bulk rejected 0 / Refresh <200ms / Flush <500ms / Merge <1s

**第一步** es_monitor_write_performance（cluster="%s", namespace="%s"）
**第二步** es_monitor_thread_pool — write/bulk rejected
**第三步** es_monitor_nodes_latency — 延迟热点节点
**第四步** es_monitor_nodes_disk — 磁盘 I/O
**第五步** es_monitor_nodes_gc — GC 暂停
**第六步** es_monitor_segments — 段合并压力

输出：写入性能评分卡 + 瓶颈定位 + 调优参数建议表（refresh_interval/translog.durability/merge.policy 等）。`, c, ns, c, ns)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES %s/%s 写入性能分析", c, ns),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 17. ES 搜索性能专项分析 =====

var promptESSearchPerformanceAnalysis = &mcp.Prompt{
	Name:        "es_search_performance_analysis",
	Description: "Elasticsearch 搜索性能专项分析：从 Query/Fetch 延迟、缓存命中率、段数量、Fielddata 内存、节点均衡性、GC 影响等维度全面分析搜索瓶颈。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
		{Name: "datasource", Description: "ES 数据源名称（可选）", Required: false},
		{Name: "slow_index", Description: "搜索慢的索引名称（可选）", Required: false},
	},
}

func handleESSearchPerformanceAnalysis(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]

	instruction := fmt.Sprintf(`请对 ES 集群 %s/%s 进行搜索性能专项分析。

阈值参考：Query延迟 <50ms / Fetch <20ms / Cache命中率 >60%%%% / search rejected 0 / 段数 <30/索引

**第一步** es_monitor_search_performance（cluster="%s", namespace="%s"）
**第二步** es_monitor_index_perf — 找出延迟最高的索引
**第三步** es_monitor_index_cache — Cache 命中率
**第四步** es_monitor_segments — 段数量
**第五步** es_monitor_node_balance — 搜索 OPS 均衡性
**第六步** es_monitor_nodes_gc — GC 影响

输出：搜索性能评分卡 + 慢查询索引 Top 10 + 优化建议（force merge/增加 Cache/优化 mapping/增加副本）。`, c, ns, c, ns)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES %s/%s 搜索性能分析", c, ns),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}
