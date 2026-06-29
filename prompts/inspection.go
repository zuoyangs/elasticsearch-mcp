package prompts

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ===== 1. 集群巡检 =====

var promptClusterInspection = &mcp.Prompt{
	Name:        "es_cluster_inspection",
	Description: "Elasticsearch 集群全面巡检：一键执行健康状态、资源使用率（CPU/内存/JVM/磁盘）、线程池、GC、断路器等全方位检查，输出标准化巡检报告。",
	Arguments: []*mcp.PromptArgument{
		{Name: "cluster", Description: "k8s 集群名称（Prometheus cluster 标签）", Required: true},
		{Name: "namespace", Description: "k8s 命名空间（同一 namespace 下只有一个 ES 集群）", Required: true},
		{Name: "datasource", Description: "ES 数据源名称（用于 ES API 查询，可选）", Required: false},
	},
}

func handleClusterInspection(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	cluster := req.Params.Arguments["cluster"]
	namespace := req.Params.Arguments["namespace"]
	datasource := req.Params.Arguments["datasource"]

	dsHint := ""
	if datasource != "" {
		dsHint = fmt.Sprintf(`，ES API 工具使用 datasource="%s"`, datasource)
	}

	instruction := fmt.Sprintf(`请对 Elasticsearch 集群进行全面巡检，目标集群信息：
- k8s 集群: %s
- 命名空间: %s%s

## 巡检步骤（严格按顺序执行）

**第一步：KPI 总览**
调用 es_monitor_kpi_overview（cluster="%s", namespace="%s"）

**第二步：JVM 堆内存检查**
调用 es_monitor_nodes_jvm_heap（cluster="%s", namespace="%s"）

**第三步：GC 检查**
调用 es_monitor_nodes_gc（cluster="%s", namespace="%s"）

**第四步：线程池检查**
调用 es_monitor_thread_pool（cluster="%s", namespace="%s"）

**第五步：读写负载**
调用 es_monitor_cluster_ops（cluster="%s", namespace="%s"）

## 输出格式
## 集群巡检报告 - %s/%s
**巡检时间**: （当前时间）
**集群状态**: 🟢 GREEN / 🟡 YELLOW / 🔴 RED

### 资源使用率
| 指标 | 当前值 | 阈值 | 状态 |
|------|--------|------|------|
| CPU | xx%%%% | 80%%%% | ✅/⚠️/❌ |
| 内存 | xx%%%% | 85%%%% | ✅/⚠️/❌ |
| JVM 堆 | xx%%%% | 85%%%% | ✅/⚠️/❌ |
| 磁盘 | xx%%%% | 85%%%% | ✅/⚠️/❌ |

### 异常项 / 风险项 / 优化建议`,
		cluster, namespace, dsHint,
		cluster, namespace,
		cluster, namespace,
		cluster, namespace,
		cluster, namespace,
		cluster, namespace,
		cluster, namespace)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("ES 集群 %s/%s 全面巡检", cluster, namespace),
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: instruction}}},
	}, nil
}
