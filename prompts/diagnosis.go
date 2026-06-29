package prompts

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ===== 2. 集群状态异常诊断 =====

var promptHealthDiagnosis = &mcp.Prompt{
	Name:        "es_health_diagnosis",
	Description: "Elasticsearch 集群状态异常诊断：当集群状态为 yellow 或 red 时，自动分析根因，定位问题索引和未分配分片，检查磁盘水位线和分片恢复进度。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
		{Name: "datasource", Description: "ES 数据源名称（用于 ES API 查询）", Required: true},
	},
}

func handleHealthDiagnosis(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	cluster := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]
	datasource := req.Params.Arguments["datasource"]

	instruction := fmt.Sprintf(`ES 集群 %s/%s 状态异常，请进行根因分析。

**第一步** 调用 es_cluster_health_analysis（datasource="%s"）
**第二步** 调用 es_cat_shards（datasource="%s"）查看 UNASSIGNED 分片
**第三步** 调用 es_monitor_cluster_disk（cluster="%s", namespace="%s"）+ es_cat_allocation（datasource="%s"）检查磁盘水位线
**第四步** 调用 es_cat_recovery（datasource="%s"）检查分片恢复进度
**第五步** 调用 es_list_tasks（datasource="%s", detailed=true）检查卡住的任务

输出：当前状态、未分配分片列表、根因分析、恢复建议。`,
		cluster, ns, datasource, datasource, cluster, ns, datasource, datasource, datasource)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES 集群 %s/%s 状态异常诊断", cluster, ns),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 5. JVM/GC 诊断 =====

var promptJVMGCDiagnosis = &mcp.Prompt{
	Name:        "es_jvm_gc_diagnosis",
	Description: "Elasticsearch JVM 堆内存与 GC 问题诊断：分析各节点 JVM 堆使用率、GC 频率和耗时、断路器状态、段内存占用，定位内存压力根因。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
	},
}

func handleJVMGCDiagnosis(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]

	instruction := fmt.Sprintf(`请诊断 ES 集群 %s/%s 的 JVM 和 GC 状况。

**第一步** es_monitor_nodes_jvm_heap（cluster="%s", namespace="%s"）— 堆使用率 >75%%%% 警告 >85%%%% 严重
**第二步** es_monitor_nodes_gc（cluster="%s", namespace="%s"）— Old GC >1次/分钟 或单次 >1s 需关注
**第三步** es_monitor_breakers（cluster="%s", namespace="%s"）— 检查 parent/fielddata/request/in_flight 断路器
**第四步** es_monitor_segments（cluster="%s", namespace="%s"）— 段数过多占用堆外内存

输出：各节点 JVM 堆使用率排行、GC 分析、断路器状态、根因和优化建议。`, c, ns, c, ns, c, ns, c, ns, c, ns)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES 集群 %s/%s JVM/GC 诊断", c, ns),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 6. 网络诊断 =====

var promptNetworkDiagnosis = &mcp.Prompt{
	Name:        "es_network_diagnosis",
	Description: "Elasticsearch 节点间网络与传输层诊断：检查 Transport 连接数、流量、分片恢复进度，排查节点间通信异常。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
		{Name: "datasource", Description: "ES 数据源名称", Required: true},
	},
}

func handleNetworkDiagnosis(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]
	ds := req.Params.Arguments["datasource"]

	instruction := fmt.Sprintf(`请诊断 ES 集群 %s/%s 的网络和传输层。

**第一步** es_monitor_nodes_network（cluster="%s", namespace="%s"）
**第二步** es_cat_recovery（datasource="%s"）
**第三步** es_monitor_nodes_latency（cluster="%s", namespace="%s"）

输出：各节点 Transport 连接数和流量、分片恢复进度、延迟异常节点、网络瓶颈分析。`, c, ns, c, ns, ds, c, ns)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES 集群 %s/%s 网络诊断", c, ns),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 7. 索引生命周期检查 =====

var promptIndexLifecycleCheck = &mcp.Prompt{
	Name:        "es_index_lifecycle_check",
	Description: "Elasticsearch 索引生命周期（ILM）全面检查：审计 ILM 策略配置、检查索引 ILM 流转状态、发现卡住的索引。",
	Arguments: []*mcp.PromptArgument{
		{Name: "datasource", Description: "ES 数据源名称", Required: true},
		{Name: "index_pattern", Description: "索引模式（可选，如 logs-*）", Required: false},
	},
}

func handleIndexLifecycleCheck(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	ds := req.Params.Arguments["datasource"]

	instruction := fmt.Sprintf(`请检查 ES 数据源 %s 的 ILM 状况。

**第一步** es_list_ilm_policies（datasource="%s"）— 列出所有 ILM 策略
**第二步** es_list_templates（datasource="%s"）— 检查模板与 ILM 关联
**第三步** es_list_indices + es_get_ilm_status — 检查 ILM 卡住的索引（step=ERROR）

输出：ILM 策略清单、模板关联、异常索引列表、优化建议。`, ds, ds, ds)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES %s ILM 检查", ds),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 11. ES 侧日志丢失排查 =====

var promptLogMissingFromES = &mcp.Prompt{
	Name:        "es_log_missing_diagnosis",
	Description: "从 Elasticsearch 存储层排查日志丢失：检查目标索引是否存在、文档数是否增长、写入是否有拒绝、ILM 是否正常。",
	Arguments: []*mcp.PromptArgument{
		{Name: "datasource", Description: "ES 数据源名称", Required: true},
		{Name: "index_pattern", Description: "目标索引模式，如 bff-user-*", Required: true},
		{Name: "cluster", Description: "k8s 集群名称（可选）", Required: false},
		{Name: "namespace", Description: "k8s 命名空间（可选）", Required: false},
	},
}

func handleLogMissingFromES(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	ds := req.Params.Arguments["datasource"]
	idxPat := req.Params.Arguments["index_pattern"]
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]

	monitorStep := ""
	if c != "" && ns != "" {
		monitorStep = fmt.Sprintf(`
**第四步** es_monitor_index_perf（cluster="%s", namespace="%s"）检查写入 OPS/延迟
**第五步** es_monitor_thread_pool（cluster="%s", namespace="%s"）检查 write rejected`, c, ns, c, ns)
	}

	instruction := fmt.Sprintf(`索引 %s（数据源: %s）日志可能丢失，请排查。

**第一步** es_list_indices（datasource="%s", pattern="%s"）确认索引存在
**第二步** es_document_count（datasource="%s"）确认文档写入
**第三步** es_cluster_health（datasource="%s"）确认集群状态%s

如果 ES 侧正常，建议检查上游 kafka-mcp 和 logstash-mcp。`, idxPat, ds, ds, idxPat, ds, ds, monitorStep)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("索引 %s 日志丢失排查", idxPat),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}
