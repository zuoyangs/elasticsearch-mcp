package prompts

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ===== 4. 磁盘容量分析 =====

var promptDiskCapacityAnalysis = &mcp.Prompt{
	Name:        "es_disk_capacity_analysis",
	Description: "Elasticsearch 磁盘容量分析与告警处理：检查各节点磁盘使用率、定位最大索引、检查 ILM 流转状态，给出容量规划建议。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
		{Name: "datasource", Description: "ES 数据源名称", Required: true},
	},
}

func handleDiskCapacityAnalysis(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]
	ds := req.Params.Arguments["datasource"]

	instruction := fmt.Sprintf(`请分析 ES 集群 %s/%s 的磁盘容量。

**第一步** es_monitor_nodes_disk（cluster="%s", namespace="%s"）— 各节点磁盘使用率（>80%%%% low / >85%%%% high / >95%%%% flood）
**第二步** es_monitor_cluster_disk（cluster="%s", namespace="%s"）— 集群总使用率
**第三步** es_cat_allocation（datasource="%s"）— 分片分配与磁盘分布
**第四步** es_list_indices（datasource="%s"）— Top 10 大索引
**第五步** es_get_ilm_status — 检查大索引 ILM 是否卡住

输出：各节点水位线状态、Top 10 大索引、ILM 异常索引、容量预测、优化建议。`, c, ns, c, ns, c, ns, ds, ds)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES %s/%s 磁盘容量分析", c, ns),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 10. 索引清理分析 =====

var promptIndexCleanupAnalysis = &mcp.Prompt{
	Name:        "es_index_cleanup_analysis",
	Description: "Elasticsearch 索引清理分析：找出过期索引、空索引、ILM 异常索引、存储占用大的索引，给出清理建议和预计释放空间。",
	Arguments: []*mcp.PromptArgument{
		{Name: "datasource", Description: "ES 数据源名称", Required: true},
	},
}

func handleIndexCleanupAnalysis(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	ds := req.Params.Arguments["datasource"]

	instruction := fmt.Sprintf(`请分析 ES 数据源 "%s" 中可以清理的索引。

**第一步** es_list_indices（datasource="%s"）— 获取所有索引
**第二步** 识别：已关闭索引、空索引、过期日期索引、red 状态索引
**第三步** es_get_ilm_status — 找出 ILM 卡住的索引

输出：建议清理的索引表（名称/大小/文档数/状态/原因/风险）+ 预计释放空间。`, ds, ds)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES %s 索引清理分析", ds),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 12. ES 成本分析 =====

var promptESCostAnalysis = &mcp.Prompt{
	Name:        "es_cost_analysis",
	Description: "Elasticsearch 集群成本分析：核算资源配置与实际利用率，按索引维度分析存储占比，给出降本建议。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
		{Name: "datasource", Description: "ES 数据源名称（可选）", Required: false},
	},
}

func handleESCostAnalysis(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]

	instruction := fmt.Sprintf(`请对 ES 集群 %s/%s 进行成本分析。

**第一步** es_monitor_cost_overview（cluster="%s", namespace="%s"）— 资源配置 vs 实际使用
**第二步** es_monitor_index_storage_topn（cluster="%s", namespace="%s"）— Top 20 大索引
**第三步** es_monitor_node_balance（cluster="%s", namespace="%s"）— 节点负载均衡

输出：资源利用率表（CPU/内存/磁盘）+ 索引存储 Top 10 占比 + 成本优化建议。`, c, ns, c, ns, c, ns, c, ns)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES %s/%s 成本分析", c, ns),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 13. ES 容量规划 =====

var promptESCapacityPlanning = &mcp.Prompt{
	Name:        "es_capacity_planning",
	Description: "Elasticsearch 集群容量规划：基于磁盘水位、存储增长趋势、索引分布，预测磁盘满时间，给出扩容方案对比。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
		{Name: "datasource", Description: "ES 数据源名称（可选）", Required: false},
	},
}

func handleESCapacityPlanning(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]

	instruction := fmt.Sprintf(`请对 ES 集群 %s/%s 进行容量规划。

**第一步** es_monitor_capacity_watermark（cluster="%s", namespace="%s"）— 各节点磁盘使用率 + 1h/24h 增长量
**第二步** es_monitor_index_storage_topn（cluster="%s", namespace="%s"）— 存储最大和增长最快的索引
**第三步** es_monitor_nodes_disk（cluster="%s", namespace="%s"）— 各节点磁盘详情

输出：当前容量状态表 + 增长趋势（预计 N 天后达到 85%%%% watermark）+ 扩容方案对比表（调整ILM/清理索引/冷热分离/扩磁盘/加节点）。`, c, ns, c, ns, c, ns, c, ns)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES %s/%s 容量规划", c, ns),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}

// ===== 15. ES 分片容量管理分析 =====

var promptESShardCapacityManagement = &mcp.Prompt{
	Name:        "es_shard_capacity_management",
	Description: "Elasticsearch 分片容量管理分析：围绕节点数量、分片大小、分片数量的联动关系，评估当前分片规格是否合理，给出扩容路径、容量计算公式、风险阈值和汇报话术。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称", Required: true},
		{Name: "namespace", Description: "k8s 命名空间", Required: true},
		{Name: "datasource", Description: "ES 数据源名称", Required: true},
	},
}

func handleESShardCapacityManagement(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	c := req.Params.Arguments["cluster"]
	ns := req.Params.Arguments["namespace"]
	ds := req.Params.Arguments["datasource"]

	instruction := fmt.Sprintf(`请对 ES 集群 %s/%s 进行分片容量管理分析。

## 方法论
- 单分片 30-50GB（最佳），<10GB 过小，>50GB 过大，>100GB 严重风险
- 总分片数 = 节点数 x 1.5~3 倍
- 每节点分片数 <= JVM堆(GB) x 20

## 数据采集
**第一步** es_cluster_stats（datasource="%s"）— 节点数/总分片数/总存储
**第二步** es_monitor_node_balance（cluster="%s", namespace="%s"）— 各节点分片数/存储/CPU/JVM
**第三步** es_monitor_index_shards（cluster="%s", namespace="%s"）— 超大分片/副本不足
**第四步** es_monitor_index_storage_topn（cluster="%s", namespace="%s"）— Top 20 大索引
**第五步** es_list_indices（datasource="%s"）— 各索引分片数/副本数/大小
**第六步** es_monitor_capacity_watermark（cluster="%s", namespace="%s"）— 增长趋势

## 输出
1. 集群分片概览表（总分片数/每节点分片数/平均分片大小 vs 建议范围）
2. 分片规格异常索引表
3. 扩容决策矩阵（磁盘>80%%/磁盘>85%%/单分片>50GB/总分片>节点x3/JVM>85%%）
4. 容量计算（日增存储/剩余/预计 N 天后达到 watermark）
5. 汇报话术模板`, c, ns, ds, c, ns, c, ns, c, ns, ds, c, ns)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES %s/%s 分片容量管理分析", c, ns),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}
