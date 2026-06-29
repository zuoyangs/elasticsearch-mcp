// Package tools - monitoring.go 提供基于 Thanos/Prometheus 指标的 Elasticsearch 场景化监控工具。
// 这些工具对应 Grafana Dashboard 中的各个面板，通过 PromQL 查询 Thanos Query API 获取实时监控数据。
// 用户需要提供 cluster（k8s 集群名）和 namespace（ES 所在命名空间）参数来定位目标集群。
// 同一个 namespace 下只会有一个 ES 集群，因此 cluster + namespace 即可唯一定位。
package tools

import (
	"context"
	"fmt"

	"elasticsearch-mcp/thanos"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MonitoringTools 基于 Thanos 的 Elasticsearch 监控工具集
type MonitoringTools struct {
	thanosClient *thanos.Client
}

// NewMonitoringTools 创建监控工具集
func NewMonitoringTools(thanosClient *thanos.Client) *MonitoringTools {
	return &MonitoringTools{thanosClient: thanosClient}
}

// clusterSchema 返回通用的集群参数 schema（cluster + namespace 唯一定位 ES 集群）
func clusterSchema() map[string]*jsonschema.Schema {
	return map[string]*jsonschema.Schema{
		"cluster":   {Type: "string", Description: "k8s 集群名称（必填，对应 Prometheus 中的 cluster 标签）"},
		"namespace": {Type: "string", Description: "k8s 命名空间（必填，ES 所在的 namespace，同一 namespace 下只有一个 ES 集群）"},
	}
}

// clusterWithNodeSchema 返回带节点过滤的集群参数 schema
func clusterWithNodeSchema() map[string]*jsonschema.Schema {
	s := clusterSchema()
	s["name"] = &jsonschema.Schema{Type: "string", Description: "节点名称过滤（可选，支持正则，如 es-data-.*）"}
	return s
}

// GetMonitoringTools 返回所有监控工具定义
func GetMonitoringTools() []mcp.Tool {
	return []mcp.Tool{
		// ===== KPI 概览 =====
		{
			Name: "foundation.elasticsearch.monitor-kpi-overview",
			Description: `一次性获取 ES 集群的核心 KPI 指标：集群健康状态、断路器触发次数、待处理任务数、关键资源使用率（CPU/内存/JVM/磁盘）、节点数、分片状态、文件句柄总数。对应 Grafana Dashboard 顶部 KPI 面板。
当需要进行日常巡检、快速了解集群整体健康状况、SRE 告警响应时使用。不适用于深度根因分析（请用 monitor-node-balance 或 monitor-write-performance 等专项工具）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选，支持正则）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]，PromQL 执行失败会返回 [ES_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Cluster Overview =====
		{
			Name: "foundation.elasticsearch.monitor-cluster-disk",
			Description: `获取集群磁盘总用量和总使用率。磁盘使用率超过 80%（low watermark）时 ES 停止分配新分片，超过 85%（high watermark）时开始迁移分片，超过 95%（flood stage）时索引变为只读。
当需要容量规划和磁盘告警响应、排查分片无法分配是否因磁盘水位线导致时使用。不适用于节点级磁盘详情（请用 monitor-nodes-disk）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Search/Indexing/Translog/Refresh/Flush/Merge =====
		{
			Name: "foundation.elasticsearch.monitor-cluster-ops",
			Description: `获取集群级别的核心操作速率：搜索 Query/Fetch OPS、写入 Indexing OPS、Translog OPS、Refresh OPS、Flush OPS、Merge OPS。用于评估集群整体负载水平，识别读写流量高峰，判断是否需要扩容。
当需要评估集群负载水平、识别读写流量高峰、判断是否需要扩容时使用。不适用于索引级或节点级操作速率（请用 monitor-index-perf）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== ThreadPool =====
		{
			Name: "foundation.elasticsearch.monitor-thread-pool",
			Description: `获取各节点线程池的活跃线程数、队列长度、拒绝次数（write/search/bulk/get/snapshot 等）。线程池拒绝（rejected > 0）是 ES 过载的关键信号，write 线程池拒绝导致 429 错误，search 线程池拒绝导致搜索超时。
当需要排查写入被拒绝（429 错误）、搜索超时等性能问题时使用。不适用于断路器状态（请用 monitor-breakers）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Nodes: CPU and Memory =====
		{
			Name: "foundation.elasticsearch.monitor-nodes-cpu-memory",
			Description: `获取各节点的 CPU 使用率、JVM 堆内存使用量/使用率/已提交量、操作系统内存使用率。CPU 持续 >85% 或 JVM 堆 >75% 是性能劣化的前兆。
当需要定位资源热点节点、评估是否需要扩容、排查性能劣化根因时使用。不适用于 JVM GC 详情（请用 monitor-nodes-gc）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Nodes: JVM GC =====
		{
			Name: "foundation.elasticsearch.monitor-nodes-gc",
			Description: `获取各节点的 GC 次数和 GC 耗时（young/old 分代）。频繁的 Old GC 或单次 GC 耗时过长（>1s）会导致 stop-the-world 暂停，直接影响搜索和写入延迟。
当需要排查 JVM GC 导致的性能问题、评估 GC 频率是否正常时使用。不适用于 JVM 堆内存使用率（请用 monitor-nodes-jvm-heap）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Nodes: JVM Heap =====
		{
			Name: "foundation.elasticsearch.monitor-nodes-jvm-heap",
			Description: `获取各节点 JVM 堆内存使用量、使用率、5 分钟平均使用率。堆内存使用率持续 >75% 需要关注，>85% 需要紧急处理（可能触发 OOM 或频繁 Full GC）。
当需要监控 JVM 堆内存水位、排查 OOM 风险时使用。不适用于 JVM 堆分区详情（请用 monitor-nodes-jvm-heap-details）或 GC 统计（请用 monitor-nodes-gc）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Nodes: Disk =====
		{
			Name: "foundation.elasticsearch.monitor-nodes-disk",
			Description: `获取各节点的磁盘使用率、磁盘用量、磁盘设备读写速率和读写 IOPS。用于定位磁盘 I/O 瓶颈节点、评估 SSD/HDD 性能差异、排查写入延迟高的根因。
当需要排查磁盘 I/O 瓶颈、评估磁盘性能、定位写入延迟高的节点时使用。不适用于集群级磁盘概览（请用 monitor-cluster-disk）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Nodes: Breakers =====
		{
			Name: "foundation.elasticsearch.monitor-breakers",
			Description: `获取各节点各类断路器（fielddata/request/in_flight_requests/parent 等）的触发次数、当前预估内存、限制值。断路器触发意味着某类操作的内存使用超过阈值被熔断，常见于大聚合查询或并发请求过多。
当需要排查断路器触发原因、评估内存限制是否合理时使用。不适用于线程池状态（请用 monitor-thread-pool）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Index-level Search/Indexing =====
		{
			Name: "foundation.elasticsearch.monitor-index-perf",
			Description: `获取各索引的搜索 Query/Fetch OPS、写入 Indexing OPS，以及对应的延迟。用于定位慢查询索引和写入热点索引。
当需要定位慢查询索引、写入热点索引、评估索引级读写性能时使用。不适用于缓存分析（请用 monitor-index-cache）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Index Cache =====
		{
			Name: "foundation.elasticsearch.monitor-index-cache",
			Description: `获取 Query Cache 大小、命中数、未命中数、驱逐数，以及 Fielddata 内存使用量和变化速率。缓存命中率低说明查询模式不适合缓存或缓存空间不足，Fielddata 内存持续增长可能导致断路器触发。
当需要评估缓存命中率、排查 Fielddata 内存增长问题时使用。不适用于索引读写性能（请用 monitor-index-perf）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Nodes: Network =====
		{
			Name: "foundation.elasticsearch.monitor-nodes-network",
			Description: `获取各节点的 Transport 层连接数、发送/接收字节速率、发送/接收数据包速率。Transport 层是节点间通信的核心通道，网络瓶颈会导致分片恢复慢、跨节点搜索延迟高。
当需要排查节点间通信瓶颈、分片恢复慢、跨节点搜索延迟高时使用。不适用于磁盘 I/O（请用 monitor-nodes-disk）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Nodes: Segments =====
		{
			Name: "foundation.elasticsearch.monitor-segments",
			Description: `获取各节点的 Segment 数量、Segment 内存占用、文件描述符使用数/最大数。段数过多会降低搜索性能并增加堆外内存占用，文件描述符耗尽会导致索引操作失败。
当需要诊断段合并问题、评估文件描述符使用、排查搜索性能下降时使用。不适用于索引级维护操作（请用 monitor-index-maintenance-ops）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Nodes: Query/Indexing/Merging Latency =====
		{
			Name: "foundation.elasticsearch.monitor-nodes-latency",
			Description: `获取各节点的搜索查询延迟、写入延迟、段合并延迟的汇总统计。用于定位延迟热点节点，判断是否存在单节点性能劣化导致的整体延迟升高。
当需要定位延迟热点节点、排查单节点性能劣化时使用。不适用于索引级延迟（请用 monitor-index-perf）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Cluster: Indices =====
		{
			Name: "foundation.elasticsearch.monitor-cluster-indices",
			Description: `获取集群的索引总数量、索引总大小（全量/主分片）、各索引的存储大小和文档数排行。用于存储容量审计、识别大索引、评估数据增长趋势。
当需要存储容量审计、识别大索引、评估数据增长趋势时使用。不适用于索引文档数增长监控（请用 monitor-index-doc-count）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Indices: Document Count =====
		{
			Name: "foundation.elasticsearch.monitor-index-doc-count",
			Description: `获取各索引的主分片文档数、全量文档数及其增长速率。用于监控数据写入是否正常、发现文档数异常增长或停滞的索引、评估数据保留策略效果。
当需要监控数据写入趋势、发现文档数异常的索引时使用。不适用于索引存储大小（请用 monitor-index-store-size）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Indices: Store Size =====
		{
			Name: "foundation.elasticsearch.monitor-index-store-size",
			Description: `获取各索引的主分片存储大小、全量存储大小及其增长速率。用于识别存储增长最快的索引、评估磁盘容量消耗趋势、发现异常膨胀的索引。
当需要识别存储增长最快的索引、评估磁盘容量消耗趋势时使用。不适用于文档数监控（请用 monitor-index-doc-count）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Indices: Refresh | Flush | Merge =====
		{
			Name: "foundation.elasticsearch.monitor-index-maintenance-ops",
			Description: `获取各索引的 Refresh OPS/延迟、Flush OPS/延迟、Merge OPS/延迟。Refresh 频率影响搜索实时性，Flush 频率影响 translog 大小，Merge 频率影响 I/O 和 CPU。
当需要评估索引维护操作频率、排查 Refresh/Flush/Merge 导致的性能问题时使用。不适用于节点级维护操作（请用 monitor-nodes-maintenance）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Indices: Memory Details =====
		{
			Name: "foundation.elasticsearch.monitor-index-memory-details",
			Description: `获取各索引的内存细分：Doc Values 内存、Fields 内存、Fixed Bit Set 内存、Index Writer 内存、Norms 内存、Points 内存、Terms 内存、Version Map 内存。用于诊断内存占用过高的根因，定位哪类数据结构消耗了最多堆内存。
当需要诊断索引内存占用过高的根因时使用。不适用于 JVM 堆内存监控（请用 monitor-nodes-jvm-heap）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Indices: Shards =====
		{
			Name: "foundation.elasticsearch.monitor-index-shards",
			Description: `获取各索引的分片大小分布、超大分片（>25GB）检测、副本数检测（副本数 <1 的索引无容灾能力）。分片大小不均衡会导致热点节点，单分片过大（>50GB）会影响恢复速度。
当需要排查分片大小不均衡、超大分片、副本数不足的索引时使用。不适用于节点级分片分配（请用 list-allocation）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Nodes: Refresh | Merge | Flush =====
		{
			Name: "foundation.elasticsearch.monitor-nodes-maintenance",
			Description: `获取各节点的 Refresh 耗时、Merge 耗时、Flush 耗时。用于定位维护操作耗时异常的节点（通常与磁盘 I/O 性能相关）。
当需要定位维护操作耗时异常的节点、排查磁盘 I/O 性能问题时使用。不适用于索引级维护操作（请用 monitor-index-maintenance-ops）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Nodes: JVM Heap Details =====
		{
			Name: "foundation.elasticsearch.monitor-nodes-jvm-heap-details",
			Description: `获取各节点 JVM 堆内存的分区使用情况（Young/Old/Survivor），用于分析内存分配模式和 GC 压力来源。
当需要分析 JVM 内存分配模式、GC 压力来源时使用。不适用于 JVM 堆总使用率（请用 monitor-nodes-jvm-heap）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== Nodes: Topology =====
		{
			Name: "foundation.elasticsearch.monitor-nodes-topology",
			Description: `获取各节点的角色（master/data/ingest/coordinating）、OS 信息、JVM 版本、ES 版本、文档数、存储大小。用于集群拓扑审计、确认节点角色分配是否合理、版本一致性检查。
当需要进行集群拓扑审计、确认节点角色分配、版本一致性检查时使用。不适用于节点运行时指标（请用 monitor-nodes-cpu-memory）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},

		// ===== 成本/容量/性能 =====
		{
			Name: "foundation.elasticsearch.monitor-cost-overview",
			Description: `获取 ES 集群的资源配置（CPU/内存/存储/副本数/节点集）与实际使用量对比。用于成本核算、资源利用率评估、识别过度配置或配置不足的维度。
当需要进行成本核算、资源利用率评估时使用。不适用于容量水位趋势（请用 monitor-capacity-watermark）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},
		{
			Name: "foundation.elasticsearch.monitor-capacity-watermark",
			Description: `获取各节点磁盘使用率/剩余量、集群总存储使用率、索引存储增长速率（1h/24h）、文档增长速率。用于容量规划、预测磁盘满时间、触发扩容决策。ES 磁盘 >80% low watermark 停止分配分片，>85% high watermark 开始迁移，>95% flood stage 索引只读。
当需要容量规划、预测磁盘满时间、触发扩容决策时使用。不适用于成本概览（请用 monitor-cost-overview）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},
		{
			Name: "foundation.elasticsearch.monitor-index-storage-topn",
			Description: `获取存储占用最大的 Top 20 索引、增长最快的 Top 20 索引、文档数最多的 Top 20 索引。用于识别存储大户、评估各索引的成本占比、发现异常增长的索引。
当需要识别存储大户、发现异常增长的索引时使用。不适用于集群级存储概览（请用 monitor-cluster-disk）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},
		{
			Name: "foundation.elasticsearch.monitor-write-performance",
			Description: `获取集群和各节点的写入 Indexing OPS/延迟、Bulk 拒绝数、Translog 大小/OPS、Refresh/Flush/Merge 延迟。用于定位写入瓶颈（磁盘 I/O？GC？线程池饱和？Merge 压力？）。
当需要定位写入瓶颈、排查写入性能问题时使用。不适用于搜索性能（请用 monitor-search-performance）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},
		{
			Name: "foundation.elasticsearch.monitor-search-performance",
			Description: `获取集群和各节点的搜索 Query/Fetch OPS/延迟、Query Cache 命中率、Fielddata 内存、段数量。用于定位搜索瓶颈（段数过多？缓存命中率低？GC 暂停？节点负载不均？）。
当需要定位搜索瓶颈、排查搜索性能问题时使用。不适用于写入性能（请用 monitor-write-performance）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},
		{
			Name: "foundation.elasticsearch.monitor-node-balance",
			Description: `获取各节点的分片数、存储大小、CPU 使用率、JVM 堆使用率、搜索/写入 OPS 对比。用于发现热点节点、评估分片分配是否均衡、指导分片重分配。
当需要发现热点节点、评估分片分配是否均衡时使用。不适用于单节点详细信息（请用 monitor-nodes-cpu-memory）。
参数 cluster 为必填，为 k8s 集群名称；参数 namespace 为必填，为 ES 所在命名空间；参数 name 为节点名称过滤（可选）。
如 Thanos 查询失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: clusterWithNodeSchema(),
				Required:   []string{"cluster", "namespace"},
			},
		},
	}
}

// HandleMonitoringTool 处理监控工具调用
func (mt *MonitoringTools) HandleMonitoringTool(ctx context.Context, toolName string, args map[string]interface{}) mcp.CallToolResult {
	switch toolName {
	case "foundation.elasticsearch.monitor-kpi-overview":
		return mt.handleKPIOverview(ctx, args)
	case "foundation.elasticsearch.monitor-cluster-disk":
		return mt.handleClusterDisk(ctx, args)
	case "foundation.elasticsearch.monitor-cluster-ops":
		return mt.handleClusterOps(ctx, args)
	case "foundation.elasticsearch.monitor-thread-pool":
		return mt.handleThreadPool(ctx, args)
	case "foundation.elasticsearch.monitor-nodes-cpu-memory":
		return mt.handleNodesCPUMemory(ctx, args)
	case "foundation.elasticsearch.monitor-nodes-gc":
		return mt.handleNodesGC(ctx, args)
	case "foundation.elasticsearch.monitor-nodes-jvm-heap":
		return mt.handleNodesJVMHeap(ctx, args)
	case "foundation.elasticsearch.monitor-nodes-disk":
		return mt.handleNodesDisk(ctx, args)
	case "foundation.elasticsearch.monitor-breakers":
		return mt.handleBreakers(ctx, args)
	case "foundation.elasticsearch.monitor-index-perf":
		return mt.handleIndexPerf(ctx, args)
	case "foundation.elasticsearch.monitor-index-cache":
		return mt.handleIndexCache(ctx, args)
	case "foundation.elasticsearch.monitor-nodes-network":
		return mt.handleNodesNetwork(ctx, args)
	case "foundation.elasticsearch.monitor-segments":
		return mt.handleSegments(ctx, args)
	case "foundation.elasticsearch.monitor-nodes-latency":
		return mt.handleNodesLatency(ctx, args)
	case "foundation.elasticsearch.monitor-cluster-indices":
		return mt.handleClusterIndices(ctx, args)
	case "foundation.elasticsearch.monitor-index-doc-count":
		return mt.handleIndexDocCount(ctx, args)
	case "foundation.elasticsearch.monitor-index-store-size":
		return mt.handleIndexStoreSize(ctx, args)
	case "foundation.elasticsearch.monitor-index-maintenance-ops":
		return mt.handleIndexMaintenanceOps(ctx, args)
	case "foundation.elasticsearch.monitor-index-memory-details":
		return mt.handleIndexMemoryDetails(ctx, args)
	case "foundation.elasticsearch.monitor-index-shards":
		return mt.handleIndexShards(ctx, args)
	case "foundation.elasticsearch.monitor-nodes-maintenance":
		return mt.handleNodesMaintenance(ctx, args)
	case "foundation.elasticsearch.monitor-nodes-jvm-heap-details":
		return mt.handleNodesJVMHeapDetails(ctx, args)
	case "foundation.elasticsearch.monitor-nodes-topology":
		return mt.handleNodesTopology(ctx, args)
	case "foundation.elasticsearch.monitor-cost-overview":
		return mt.handleCostOverview(ctx, args)
	case "foundation.elasticsearch.monitor-capacity-watermark":
		return mt.handleCapacityWatermark(ctx, args)
	case "foundation.elasticsearch.monitor-index-storage-topn":
		return mt.handleIndexStorageTopN(ctx, args)
	case "foundation.elasticsearch.monitor-write-performance":
		return mt.handleWritePerformance(ctx, args)
	case "foundation.elasticsearch.monitor-search-performance":
		return mt.handleSearchPerformance(ctx, args)
	case "foundation.elasticsearch.monitor-node-balance":
		return mt.handleNodeBalance(ctx, args)
	default:
		return createErrorResult(fmt.Sprintf("未知监控工具: %s", toolName))
	}
}

// queryThanos 执行 PromQL 查询并返回结果
func (mt *MonitoringTools) queryThanos(ctx context.Context, query string) (interface{}, error) {
	result, err := mt.thanosClient.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	if result.Status != "success" {
		return nil, fmt.Errorf("查询失败: %s (%s)", result.Error, result.ErrorType)
	}
	return result.Data, nil
}

// multiQuery 批量执行多个 PromQL 查询，返回 map[label]result
func (mt *MonitoringTools) multiQuery(ctx context.Context, queries map[string]string) map[string]interface{} {
	results := make(map[string]interface{})
	for label, query := range queries {
		data, err := mt.queryThanos(ctx, query)
		if err != nil {
			results[label] = map[string]interface{}{"error": err.Error()}
		} else {
			results[label] = data
		}
	}
	return results
}

// ===== PromQL 过滤条件简写 =====
// cn = cluster + namespace 过滤: cluster="x",namespace="y"
// cnn = cluster + namespace + name 过滤: cluster="x",namespace="y",name=~"z"

func cn(cluster, ns string) string {
	return fmt.Sprintf(`cluster="%s",namespace="%s"`, cluster, ns)
}

func cnn(cluster, ns, name string) string {
	return fmt.Sprintf(`cluster="%s",namespace="%s",name=~"%s"`, cluster, ns, name)
}

// ===== KPI Overview =====
func (mt *MonitoringTools) handleKPIOverview(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	f := cn(cluster, ns)
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"cluster_health":      fmt.Sprintf(`elasticsearch_cluster_health_status{%s,color="red"}==1 or (elasticsearch_cluster_health_status{%s,color="green"}==1)+4 or (elasticsearch_cluster_health_status{%s,color="yellow"}==1)+2`, f, f, f),
		"breakers_tripped":    fmt.Sprintf(`sum(rate(elasticsearch_breakers_tripped{%s}[5m])) by (breaker)`, fn),
		"pending_tasks":       fmt.Sprintf(`elasticsearch_cluster_health_number_of_pending_tasks{%s}`, f),
		"cpu_usage":           fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s,pod=~".*-es-.*",image!="",pod!=""}[2m])) / sum(kube_pod_container_resource_limits{%s,pod=~".*-es-.*",resource="cpu"})`, f, f),
		"memory_usage":        fmt.Sprintf(`sum(container_memory_rss{%s,pod=~".*-es-.*",image!="",pod!="",container!="POD"}) / sum(container_spec_memory_limit_bytes{%s,pod=~".*-es-.*"})`, f, f),
		"jvm_usage":           fmt.Sprintf(`sum(elasticsearch_jvm_memory_used_bytes{%s}) / sum(elasticsearch_jvm_memory_max_bytes{%s})`, fn, fn),
		"disk_usage":          fmt.Sprintf(`1 - sum(elasticsearch_filesystem_data_available_bytes{%s}) / sum(elasticsearch_filesystem_data_size_bytes{%s})`, f, f),
		"nodes_total":         fmt.Sprintf(`elasticsearch_cluster_health_number_of_nodes{%s}`, f),
		"nodes_data":          fmt.Sprintf(`elasticsearch_cluster_health_number_of_data_nodes{%s}`, f),
		"shards_initializing": fmt.Sprintf(`elasticsearch_cluster_health_initializing_shards{%s}`, f),
		"shards_relocating":   fmt.Sprintf(`elasticsearch_cluster_health_relocating_shards{%s}`, f),
		"shards_unassigned":   fmt.Sprintf(`elasticsearch_cluster_health_unassigned_shards{%s}`, f),
		"open_files":          fmt.Sprintf(`sum(elasticsearch_process_open_files_count{%s})`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' KPI 总览", cluster, ns), results)
}

// ===== Cluster Disk =====
func (mt *MonitoringTools) handleClusterDisk(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	f := cn(cluster, ns)
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"disk_used_bytes":    fmt.Sprintf(`sum(elasticsearch_filesystem_data_size_bytes{%s} - elasticsearch_filesystem_data_available_bytes{%s})`, fn, fn),
		"disk_usage_percent": fmt.Sprintf(`1 - sum(elasticsearch_filesystem_data_available_bytes{%s}) / sum(elasticsearch_filesystem_data_size_bytes{%s})`, f, f),
		"disk_per_node":      fmt.Sprintf(`(elasticsearch_filesystem_data_size_bytes{%s} - elasticsearch_filesystem_data_available_bytes{%s}) / elasticsearch_filesystem_data_size_bytes{%s}`, fn, fn, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 磁盘监控", cluster, ns), results)
}

// ===== Cluster OPS =====
func (mt *MonitoringTools) handleClusterOps(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"search_query_ops": fmt.Sprintf(`sum(rate(elasticsearch_indices_search_query_total{%s}[2m]))`, fn),
		"search_fetch_ops": fmt.Sprintf(`sum(rate(elasticsearch_indices_search_fetch_total{%s}[2m]))`, fn),
		"indexing_ops":     fmt.Sprintf(`sum(rate(elasticsearch_indices_indexing_index_total{%s}[2m]))`, fn),
		"refresh_ops":      fmt.Sprintf(`sum(rate(elasticsearch_indices_refresh_total{%s}[2m]))`, fn),
		"flush_ops":        fmt.Sprintf(`sum(rate(elasticsearch_indices_flush_total{%s}[2m]))`, fn),
		"merges_ops":       fmt.Sprintf(`sum(rate(elasticsearch_indices_merges_total{%s}[2m]))`, fn),
		"translog_ops":     fmt.Sprintf(`sum(rate(elasticsearch_indices_translog_operations{%s}[2m]))`, fn),
		"translog_size":    fmt.Sprintf(`sum(elasticsearch_indices_translog_size_in_bytes{%s})`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 操作速率", cluster, ns), results)
}

// ===== Thread Pool =====
func (mt *MonitoringTools) handleThreadPool(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"thread_pool_active":   fmt.Sprintf(`elasticsearch_thread_pool_active_count{%s}`, fn),
		"thread_pool_queue":    fmt.Sprintf(`elasticsearch_thread_pool_queue_count{%s}`, fn),
		"thread_pool_rejected": fmt.Sprintf(`rate(elasticsearch_thread_pool_rejected_count{%s}[5m])`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 线程池状态", cluster, ns), results)
}

// ===== Nodes CPU & Memory =====
func (mt *MonitoringTools) handleNodesCPUMemory(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"cpu_percent":           fmt.Sprintf(`elasticsearch_process_cpu_percent{%s}`, fn),
		"jvm_memory_used":      fmt.Sprintf(`elasticsearch_jvm_memory_used_bytes{%s}`, fn),
		"jvm_memory_max":       fmt.Sprintf(`elasticsearch_jvm_memory_max_bytes{%s}`, fn),
		"jvm_memory_committed": fmt.Sprintf(`elasticsearch_jvm_memory_committed_bytes{%s}`, fn),
		"jvm_usage_percent":    fmt.Sprintf(`elasticsearch_jvm_memory_used_bytes{%s} / elasticsearch_jvm_memory_max_bytes{%s}`, fn, fn),
		"os_memory_used_percent": fmt.Sprintf(`elasticsearch_os_mem_used_percent{%s}`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 节点 CPU 与内存", cluster, ns), results)
}

// ===== Nodes GC =====
func (mt *MonitoringTools) handleNodesGC(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"gc_young_count": fmt.Sprintf(`rate(elasticsearch_jvm_gc_collection_seconds_count{%s,gc="young"}[5m])`, fn),
		"gc_old_count":   fmt.Sprintf(`rate(elasticsearch_jvm_gc_collection_seconds_count{%s,gc="old"}[5m])`, fn),
		"gc_young_time":  fmt.Sprintf(`rate(elasticsearch_jvm_gc_collection_seconds_sum{%s,gc="young"}[5m])`, fn),
		"gc_old_time":    fmt.Sprintf(`rate(elasticsearch_jvm_gc_collection_seconds_sum{%s,gc="old"}[5m])`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' JVM GC 统计", cluster, ns), results)
}

// ===== Nodes JVM Heap =====
func (mt *MonitoringTools) handleNodesJVMHeap(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"heap_used":          fmt.Sprintf(`elasticsearch_jvm_memory_used_bytes{%s,area="heap"}`, fn),
		"heap_max":           fmt.Sprintf(`elasticsearch_jvm_memory_max_bytes{%s,area="heap"}`, fn),
		"heap_usage_percent": fmt.Sprintf(`elasticsearch_jvm_memory_used_bytes{%s,area="heap"} / elasticsearch_jvm_memory_max_bytes{%s,area="heap"}`, fn, fn),
		"heap_usage_avg_5m":  fmt.Sprintf(`avg_over_time(elasticsearch_jvm_memory_used_bytes{%s,area="heap"}[5m]) / elasticsearch_jvm_memory_max_bytes{%s,area="heap"}`, fn, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' JVM 堆内存详情", cluster, ns), results)
}

// ===== Nodes Disk =====
func (mt *MonitoringTools) handleNodesDisk(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"disk_usage_percent": fmt.Sprintf(`1 - (elasticsearch_filesystem_data_available_bytes{%s} / elasticsearch_filesystem_data_size_bytes{%s})`, fn, fn),
		"disk_used_bytes":    fmt.Sprintf(`elasticsearch_filesystem_data_size_bytes{%s} - elasticsearch_filesystem_data_available_bytes{%s}`, fn, fn),
		"disk_total_bytes":   fmt.Sprintf(`elasticsearch_filesystem_data_size_bytes{%s}`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 节点磁盘详情", cluster, ns), results)
}

// ===== Breakers =====
func (mt *MonitoringTools) handleBreakers(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"breaker_tripped_rate":    fmt.Sprintf(`rate(elasticsearch_breakers_tripped{%s}[5m])`, fn),
		"breaker_estimated_size":  fmt.Sprintf(`elasticsearch_breakers_estimated_size_in_bytes{%s}`, fn),
		"breaker_limit_size":      fmt.Sprintf(`elasticsearch_breakers_limit_size_in_bytes{%s}`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 断路器状态", cluster, ns), results)
}

// ===== Index Performance =====
func (mt *MonitoringTools) handleIndexPerf(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"search_query_ops_by_index": fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_search_query_total{%s}[2m]))`, fn),
		"search_fetch_ops_by_index": fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_search_fetch_total{%s}[2m]))`, fn),
		"indexing_ops_by_index":     fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_indexing_index_total{%s}[2m]))`, fn),
		"search_query_latency":     fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_search_query_time_seconds_total{%s}[2m]))`, fn),
		"search_fetch_latency":     fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_search_fetch_time_seconds_total{%s}[2m]))`, fn),
		"indexing_latency":         fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_indexing_index_time_seconds_total{%s}[2m]))`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 索引级搜索/写入性能", cluster, ns), results)
}

// ===== Index Cache =====
func (mt *MonitoringTools) handleIndexCache(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"query_cache_size":      fmt.Sprintf(`elasticsearch_indices_query_cache_cache_size{%s}`, fn),
		"query_cache_hits":      fmt.Sprintf(`rate(elasticsearch_indices_query_cache_hit_count{%s}[5m])`, fn),
		"query_cache_misses":    fmt.Sprintf(`rate(elasticsearch_indices_query_cache_miss_count{%s}[5m])`, fn),
		"query_cache_evictions": fmt.Sprintf(`rate(elasticsearch_indices_query_cache_evictions{%s}[5m])`, fn),
		"fielddata_memory":      fmt.Sprintf(`elasticsearch_indices_fielddata_memory_size_in_bytes{%s}`, fn),
		"fielddata_memory_rate": fmt.Sprintf(`rate(elasticsearch_indices_fielddata_memory_size_in_bytes{%s}[5m])`, fn),
		"request_cache_size":    fmt.Sprintf(`elasticsearch_indices_request_cache_memory_size_in_bytes{%s}`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 缓存分析", cluster, ns), results)
}

// ===== Nodes Network =====
func (mt *MonitoringTools) handleNodesNetwork(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"transport_rx_bytes_rate":   fmt.Sprintf(`rate(elasticsearch_transport_rx_size_bytes_total{%s}[2m])`, fn),
		"transport_tx_bytes_rate":   fmt.Sprintf(`rate(elasticsearch_transport_tx_size_bytes_total{%s}[2m])`, fn),
		"transport_rx_packets_rate": fmt.Sprintf(`rate(elasticsearch_transport_rx_packets_total{%s}[2m])`, fn),
		"transport_tx_packets_rate": fmt.Sprintf(`rate(elasticsearch_transport_tx_packets_total{%s}[2m])`, fn),
		"transport_server_open":     fmt.Sprintf(`elasticsearch_transport_server_open_number{%s}`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 节点网络监控", cluster, ns), results)
}

// ===== Segments =====
func (mt *MonitoringTools) handleSegments(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"segments_count":        fmt.Sprintf(`elasticsearch_indices_segments_count{%s}`, fn),
		"segments_memory":       fmt.Sprintf(`elasticsearch_indices_segments_memory_bytes{%s}`, fn),
		"open_file_descriptors": fmt.Sprintf(`elasticsearch_process_open_files_count{%s}`, fn),
		"max_file_descriptors":  fmt.Sprintf(`elasticsearch_process_max_files_descriptors{%s}`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 段与文件描述符", cluster, ns), results)
}

// ===== Nodes Latency =====
func (mt *MonitoringTools) handleNodesLatency(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"query_latency":    fmt.Sprintf(`rate(elasticsearch_indices_search_query_time_seconds{%s}[2m])`, fn),
		"fetch_latency":    fmt.Sprintf(`rate(elasticsearch_indices_search_fetch_time_seconds{%s}[2m])`, fn),
		"indexing_latency": fmt.Sprintf(`rate(elasticsearch_indices_indexing_index_time_seconds{%s}[2m])`, fn),
		"merge_latency":    fmt.Sprintf(`rate(elasticsearch_indices_merges_total_time_seconds_total{%s}[2m])`, fn),
		"refresh_latency":  fmt.Sprintf(`rate(elasticsearch_indices_refresh_time_seconds_total{%s}[2m])`, fn),
		"flush_latency":    fmt.Sprintf(`rate(elasticsearch_indices_flush_time_seconds{%s}[2m])`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 节点级延迟分析", cluster, ns), results)
}

// ===== Cluster Indices =====
func (mt *MonitoringTools) handleClusterIndices(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	f := cn(cluster, ns)

	queries := map[string]string{
		"index_count":           fmt.Sprintf(`sum(count by (index) (elasticsearch_indices_store_size_bytes_total{%s}))`, f),
		"total_store_size":      fmt.Sprintf(`sum(elasticsearch_indices_store_size_bytes_total{%s})`, f),
		"primary_store_size":    fmt.Sprintf(`sum(elasticsearch_indices_store_size_bytes_primary{%s})`, f),
		"store_size_by_index":   fmt.Sprintf(`sum by (index) (elasticsearch_indices_store_size_bytes_total{%s})`, f),
		"primary_size_by_index": fmt.Sprintf(`sum by (index) (elasticsearch_indices_store_size_bytes_primary{%s})`, f),
		"docs_by_index":         fmt.Sprintf(`sum by (index) (elasticsearch_indices_docs_primary{%s})`, f),
		"replica_count_by_index": fmt.Sprintf(`sum by (index) (round(elasticsearch_indices_store_size_bytes_total{%s} / elasticsearch_indices_store_size_bytes_primary{%s}) - 1)`, f, f),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 索引概览", cluster, ns), results)
}

// ===== Index Document Count =====
func (mt *MonitoringTools) handleIndexDocCount(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"docs_primary":      fmt.Sprintf(`elasticsearch_indices_docs_primary{%s}`, fn),
		"docs_total":        fmt.Sprintf(`elasticsearch_indices_docs_total{%s}`, fn),
		"docs_primary_rate": fmt.Sprintf(`rate(elasticsearch_indices_docs_primary{%s}[2m])`, fn),
		"docs_deleted_rate": fmt.Sprintf(`rate(elasticsearch_indices_docs_deleted{%s}[2m])`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 索引文档数监控", cluster, ns), results)
}

// ===== Index Store Size =====
func (mt *MonitoringTools) handleIndexStoreSize(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"store_size_primary":      fmt.Sprintf(`elasticsearch_indices_store_size_bytes_primary{%s}`, fn),
		"store_size_total":        fmt.Sprintf(`elasticsearch_indices_store_size_bytes_total{%s}`, fn),
		"store_size_primary_rate": fmt.Sprintf(`rate(elasticsearch_indices_store_size_bytes_primary{%s}[2m])`, fn),
		"store_size_total_rate":   fmt.Sprintf(`rate(elasticsearch_indices_store_size_bytes_total{%s}[2m])`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 索引存储大小监控", cluster, ns), results)
}

// ===== Index Maintenance Ops =====
func (mt *MonitoringTools) handleIndexMaintenanceOps(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"refresh_ops_by_index":     fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_refresh_total{%s}[2m]))`, fn),
		"flush_ops_by_index":       fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_flush_total{%s}[2m]))`, fn),
		"merge_ops_by_index":       fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_merges_total{%s}[2m]))`, fn),
		"refresh_latency_by_index": fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_refresh_time_seconds_total{%s}[2m]))`, fn),
		"flush_latency_by_index":   fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_flush_time_seconds_total{%s}[2m]))`, fn),
		"merge_latency_by_index":   fmt.Sprintf(`sum by (index) (rate(elasticsearch_index_stats_merges_total_time_seconds_total{%s}[2m]))`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 索引维护操作监控", cluster, ns), results)
}

// ===== Index Memory Details =====
func (mt *MonitoringTools) handleIndexMemoryDetails(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"doc_values_memory":    fmt.Sprintf(`elasticsearch_indices_segments_doc_values_memory_in_bytes{%s}`, fn),
		"fields_memory":        fmt.Sprintf(`elasticsearch_indices_segments_fields_memory_in_bytes{%s}`, fn),
		"fixed_bit_set_memory": fmt.Sprintf(`elasticsearch_indices_segments_fixed_bit_set_memory_in_bytes{%s}`, fn),
		"index_writer_memory":  fmt.Sprintf(`elasticsearch_indices_segments_index_writer_memory_in_bytes{%s}`, fn),
		"norms_memory":         fmt.Sprintf(`elasticsearch_indices_segments_norms_memory_in_bytes{%s}`, fn),
		"points_memory":        fmt.Sprintf(`elasticsearch_indices_segments_points_memory_in_bytes{%s}`, fn),
		"terms_memory":         fmt.Sprintf(`elasticsearch_indices_segments_terms_memory_in_bytes{%s}`, fn),
		"version_map_memory":   fmt.Sprintf(`elasticsearch_indices_segments_version_map_memory_in_bytes{%s}`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 索引内存结构详情", cluster, ns), results)
}

// ===== Index Shards =====
func (mt *MonitoringTools) handleIndexShards(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	f := cn(cluster, ns)

	queries := map[string]string{
		"shard_count_by_index":  fmt.Sprintf(`sum(group(elasticsearch_indices_shards_store_size_in_bytes{%s,primary="true"}) by (index,shard)) by (index)`, f),
		"large_shards_gt25gb":   fmt.Sprintf(`sum(elasticsearch_indices_shards_store_size_in_bytes{%s,primary="true"}) by (index,shard) > 26843545600`, f),
		"replica_count_by_index": fmt.Sprintf(`sum by (index) (round(elasticsearch_indices_store_size_bytes_total{%s} / elasticsearch_indices_store_size_bytes_primary{%s}) - 1)`, f, f),
		"no_replica_indices":    fmt.Sprintf(`(round(elasticsearch_indices_store_size_bytes_total{%s} / elasticsearch_indices_store_size_bytes_primary{%s}) - 1) < 1`, f, f),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 索引分片分析", cluster, ns), results)
}

// ===== Nodes Maintenance =====
func (mt *MonitoringTools) handleNodesMaintenance(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"refresh_time": fmt.Sprintf(`rate(elasticsearch_indices_refresh_time_seconds_total{%s}[5m])`, fn),
		"merge_time":   fmt.Sprintf(`irate(elasticsearch_indices_merges_total_time_seconds_total{%s}[5m])`, fn),
		"flush_time":   fmt.Sprintf(`rate(elasticsearch_indices_flush_time_seconds{%s}[5m])`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 节点维护操作监控", cluster, ns), results)
}

// ===== Nodes JVM Heap Details =====
func (mt *MonitoringTools) handleNodesJVMHeapDetails(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"heap_used_young":     fmt.Sprintf(`elasticsearch_jvm_memory_pool_used_in_bytes{%s,pool="young"}`, fn),
		"heap_used_old":       fmt.Sprintf(`elasticsearch_jvm_memory_pool_used_in_bytes{%s,pool="old"}`, fn),
		"heap_used_survivor":  fmt.Sprintf(`elasticsearch_jvm_memory_pool_used_in_bytes{%s,pool="survivor"}`, fn),
		"heap_max_young":      fmt.Sprintf(`elasticsearch_jvm_memory_pool_max_in_bytes{%s,pool="young"}`, fn),
		"heap_max_old":        fmt.Sprintf(`elasticsearch_jvm_memory_pool_max_in_bytes{%s,pool="old"}`, fn),
		"heap_peak_used_old":  fmt.Sprintf(`elasticsearch_jvm_memory_pool_peak_used_in_bytes{%s,pool="old"}`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' JVM 堆内存分区详情", cluster, ns), results)
}

// ===== Nodes Topology =====
func (mt *MonitoringTools) handleNodesTopology(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"node_roles":          fmt.Sprintf(`elasticsearch_node_roles{%s}`, fn),
		"node_docs":           fmt.Sprintf(`elasticsearch_indices_docs{%s}`, fn),
		"node_store_size":     fmt.Sprintf(`elasticsearch_indices_store_size_bytes_total{%s}`, fn),
		"node_jvm_max":        fmt.Sprintf(`elasticsearch_jvm_memory_max_bytes{%s}`, fn),
		"node_os_info":        fmt.Sprintf(`elasticsearch_os_info{%s}`, fn),
		"node_merge_docs_rate": fmt.Sprintf(`sum by (name) (rate(elasticsearch_indices_merges_docs_total{%s}[5m]))`, fn),
		"node_docs_deleted_rate": fmt.Sprintf(`rate(elasticsearch_indices_docs_deleted{%s}[5m])`, fn),
	}

	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 节点拓扑信息", cluster, ns), results)
}

// ===== Cost Overview =====
func (mt *MonitoringTools) handleCostOverview(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	f := cn(cluster, ns)
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"config_cpu":        fmt.Sprintf(`sum by (node_set) (sre_metrics_enhancer_midware_elasticsearch_resources_limits_cpu_in_mcores{%s}) / 1000`, f),
		"config_memory":     fmt.Sprintf(`sum by (node_set) (sre_metrics_enhancer_midware_elasticsearch_resources_limits_memory_in_bytes{%s})`, f),
		"config_storage":    fmt.Sprintf(`sum by (node_set) (sre_metrics_enhancer_midware_elasticsearch_storage_size{%s})`, f),
		"config_replicas":   fmt.Sprintf(`sum by (node_set) (sre_metrics_enhancer_midware_elasticsearch_replicas{%s})`, f),
		"actual_cpu":        fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total{%s,pod=~".*-es-.*",image!="",pod!=""}[5m])) by (pod)`, f),
		"actual_memory":     fmt.Sprintf(`sum(container_memory_rss{%s,pod=~".*-es-.*",image!="",pod!="",container!="POD"}) by (pod)`, f),
		"actual_jvm":        fmt.Sprintf(`elasticsearch_jvm_memory_used_bytes{%s}`, fn),
		"actual_jvm_max":    fmt.Sprintf(`elasticsearch_jvm_memory_max_bytes{%s}`, fn),
		"actual_disk_used":  fmt.Sprintf(`elasticsearch_filesystem_data_size_bytes{%s} - elasticsearch_filesystem_data_available_bytes{%s}`, fn, fn),
		"actual_disk_total": fmt.Sprintf(`elasticsearch_filesystem_data_size_bytes{%s}`, fn),
	}
	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 成本概览", cluster, ns), results)
}

// ===== Capacity Watermark =====
func (mt *MonitoringTools) handleCapacityWatermark(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	f := cn(cluster, ns)
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"disk_usage_pct_per_node": fmt.Sprintf(`1 - (elasticsearch_filesystem_data_available_bytes{%s} / elasticsearch_filesystem_data_size_bytes{%s})`, fn, fn),
		"disk_avail_per_node":     fmt.Sprintf(`elasticsearch_filesystem_data_available_bytes{%s}`, fn),
		"cluster_disk_usage_pct":  fmt.Sprintf(`1 - sum(elasticsearch_filesystem_data_available_bytes{%s}) / sum(elasticsearch_filesystem_data_size_bytes{%s})`, f, f),
		"total_store_size":        fmt.Sprintf(`sum(elasticsearch_indices_store_size_bytes_total{%s})`, f),
		"store_increase_1h":       fmt.Sprintf(`sum(increase(elasticsearch_indices_store_size_bytes_total{%s}[1h]))`, f),
		"store_increase_24h":      fmt.Sprintf(`sum(increase(elasticsearch_indices_store_size_bytes_total{%s}[24h]))`, f),
		"docs_increase_1h":        fmt.Sprintf(`sum(increase(elasticsearch_indices_docs_primary{%s}[1h]))`, f),
		"docs_increase_24h":       fmt.Sprintf(`sum(increase(elasticsearch_indices_docs_primary{%s}[24h]))`, f),
	}
	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 容量水位", cluster, ns), results)
}

// ===== Index Storage Top N =====
func (mt *MonitoringTools) handleIndexStorageTopN(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	f := cn(cluster, ns)

	queries := map[string]string{
		"store_size_topn":      fmt.Sprintf(`topk(20, sum by (index) (elasticsearch_indices_store_size_bytes_total{%s}))`, f),
		"store_increase_topn":  fmt.Sprintf(`topk(20, sum by (index) (increase(elasticsearch_indices_store_size_bytes_total{%s}[1h])))`, f),
		"docs_topn":            fmt.Sprintf(`topk(20, sum by (index) (elasticsearch_indices_docs_primary{%s}))`, f),
		"primary_size_topn":    fmt.Sprintf(`topk(20, sum by (index) (elasticsearch_indices_store_size_bytes_primary{%s}))`, f),
	}
	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 索引存储 Top N", cluster, ns), results)
}

// ===== Write Performance =====
func (mt *MonitoringTools) handleWritePerformance(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"indexing_ops":     fmt.Sprintf(`rate(elasticsearch_indices_indexing_index_total{%s}[2m])`, fn),
		"indexing_latency": fmt.Sprintf(`rate(elasticsearch_indices_indexing_index_time_seconds{%s}[2m])`, fn),
		"bulk_rejected":    fmt.Sprintf(`rate(elasticsearch_thread_pool_rejected_count{%s,type="write"}[5m])`, fn),
		"translog_size":    fmt.Sprintf(`elasticsearch_indices_translog_size_in_bytes{%s}`, fn),
		"translog_ops":     fmt.Sprintf(`rate(elasticsearch_indices_translog_operations{%s}[2m])`, fn),
		"refresh_latency":  fmt.Sprintf(`rate(elasticsearch_indices_refresh_time_seconds_total{%s}[2m])`, fn),
		"flush_latency":    fmt.Sprintf(`rate(elasticsearch_indices_flush_time_seconds{%s}[2m])`, fn),
		"merge_latency":    fmt.Sprintf(`rate(elasticsearch_indices_merges_total_time_seconds_total{%s}[2m])`, fn),
		"merge_docs_rate":  fmt.Sprintf(`rate(elasticsearch_indices_merges_docs_total{%s}[2m])`, fn),
	}
	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 写入性能分析", cluster, ns), results)
}

// ===== Search Performance =====
func (mt *MonitoringTools) handleSearchPerformance(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"query_ops":          fmt.Sprintf(`rate(elasticsearch_indices_search_query_total{%s}[2m])`, fn),
		"query_latency":      fmt.Sprintf(`rate(elasticsearch_indices_search_query_time_seconds{%s}[2m])`, fn),
		"fetch_ops":          fmt.Sprintf(`rate(elasticsearch_indices_search_fetch_total{%s}[2m])`, fn),
		"fetch_latency":      fmt.Sprintf(`rate(elasticsearch_indices_search_fetch_time_seconds{%s}[2m])`, fn),
		"search_rejected":    fmt.Sprintf(`rate(elasticsearch_thread_pool_rejected_count{%s,type="search"}[5m])`, fn),
		"query_cache_hits":   fmt.Sprintf(`rate(elasticsearch_indices_query_cache_hit_count{%s}[5m])`, fn),
		"query_cache_misses": fmt.Sprintf(`rate(elasticsearch_indices_query_cache_miss_count{%s}[5m])`, fn),
		"fielddata_memory":   fmt.Sprintf(`elasticsearch_indices_fielddata_memory_size_in_bytes{%s}`, fn),
		"segments_count":     fmt.Sprintf(`elasticsearch_indices_segments_count{%s}`, fn),
	}
	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 搜索性能分析", cluster, ns), results)
}

// ===== Node Balance =====
func (mt *MonitoringTools) handleNodeBalance(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	cluster := getArg(args, "cluster")
	ns := getArg(args, "namespace")
	name := nodeFilter(getArg(args, "name"))
	fn := cnn(cluster, ns, name)

	queries := map[string]string{
		"store_size_per_node":   fmt.Sprintf(`elasticsearch_indices_store_size_bytes_total{%s}`, fn),
		"docs_per_node":         fmt.Sprintf(`elasticsearch_indices_docs{%s}`, fn),
		"cpu_per_node":          fmt.Sprintf(`elasticsearch_process_cpu_percent{%s}`, fn),
		"jvm_heap_pct_per_node": fmt.Sprintf(`elasticsearch_jvm_memory_used_bytes{%s} / elasticsearch_jvm_memory_max_bytes{%s}`, fn, fn),
		"search_ops_per_node":   fmt.Sprintf(`rate(elasticsearch_indices_search_query_total{%s}[2m])`, fn),
		"indexing_ops_per_node": fmt.Sprintf(`rate(elasticsearch_indices_indexing_index_total{%s}[2m])`, fn),
		"disk_pct_per_node":     fmt.Sprintf(`1 - (elasticsearch_filesystem_data_available_bytes{%s} / elasticsearch_filesystem_data_size_bytes{%s})`, fn, fn),
	}
	results := mt.multiQuery(ctx, queries)
	return createSuccessResult(fmt.Sprintf("集群 '%s/%s' 节点负载均衡分析", cluster, ns), results)
}
