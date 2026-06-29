// Package tools 提供与 Elasticsearch 交互的 MCP 工具。
// 它通过模型上下文协议接口实现各种 Elasticsearch 操作作为 MCP 工具。
// 注意：本工具集仅提供只读和安全写入操作，不包含 delete/update/put 等危险操作。
package tools

import (
	"context"
	"fmt"
	"strings"

	"elasticsearch-mcp/elasticsearch"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// datasourceDesc 是 datasource 参数的统一描述
const datasourceDesc = "数据源名称（可选，不传则使用默认数据源，可通过 foundation.elasticsearch.list-instances 查看所有可用数据源）"

// ElasticsearchTools 表示 Elasticsearch 相关 MCP 工具的集合
type ElasticsearchTools struct {
	clients           map[string]elasticsearch.Client
	defaultDatasource string
}

// NewElasticsearchTools 使用提供的客户端创建新的 ElasticsearchTools 实例
func NewElasticsearchTools(clients map[string]elasticsearch.Client) *ElasticsearchTools {
	defaultDatasource := ""
	for name := range clients {
		defaultDatasource = name
		break
	}
	return &ElasticsearchTools{
		clients:           clients,
		defaultDatasource: defaultDatasource,
	}
}

// GetTools 返回所有可用的 Elasticsearch 工具及其模式
func (et *ElasticsearchTools) GetTools() []mcp.Tool {
	return []mcp.Tool{
		// ===== 实例与集群 =====
		{
			Name: "foundation.elasticsearch.list-instances",
			Description: `列出所有已配置的 Elasticsearch 集群数据源实例，返回数据源名称列表和默认数据源。
当需要确认当前可用的集群连接、排查多集群环境下的数据源路由问题时使用。不适用于查询集群内部状态（请用 query-cluster-info 或 query-cluster-health）。
无参数。
返回结果包含 instances（数据源名称数组）、count（数量）、default_datasource（默认数据源名称）。`,
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{},
			},
		},
		{
			Name: "foundation.elasticsearch.query-cluster-info",
			Description: `获取 Elasticsearch 集群基本信息，包括集群名称、UUID、节点名称、ES 版本号、Lucene 版本等。
当需要确认集群身份、版本兼容性检查、升级前后版本对比时使用。不适用于查询集群健康状态（请用 query-cluster-health）。
参数 datasource 为数据源名称，可选。
如集群连接失败会返回 [DATASOURCE_ERROR]，请检查数据源配置是否正确。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.query-cluster-health",
			Description: `获取集群健康状态，包括 green/yellow/red 状态、节点数、活跃分片数、未分配分片数等核心指标。
当需要实时监控集群运行状况、SRE 日常巡检和告警响应时使用。不适用于深度根因分析（请用 analyze-cluster-health）。
参数 datasource 为数据源名称，可选。
如果集群非 green，建议进一步调用 analyze-cluster-health 获取根因分析。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.analyze-cluster-health",
			Description: `深度分析集群健康状态变化的根因，特别是从 green 变为 yellow 或 red 的原因。自动执行以下诊断：1) 调用 allocation/explain 获取未分配分片精确原因；2) 按索引聚合未分配分片；3) 检查各节点磁盘水位线（80%/85%/95%）；4) 对 red 索引逐个分析主分片无法分配的原因。
当集群健康状态非 green、收到集群告警、需要定位分片分配异常根因时使用。不适用于简单的健康状态查询（请用 query-cluster-health）。
参数 datasource 为数据源名称，可选。
大规模集群（几百节点、20w+ 分片）下输出会自动截断，仅展示前 30 个非 green 索引。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.query-cluster-stats",
			Description: `获取集群级别的综合统计信息，包括索引总数、文档总数、存储总量、节点角色分布、JVM 内存使用、操作系统资源等。
当需要进行集群容量规划、资源使用率评估、定期巡检报告生成时使用。不适用于节点级别统计（请用 query-nodes-stats）。
参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.query-nodes-info",
			Description: `获取集群节点详细信息，包括 JVM 配置、操作系统信息、已安装插件、节点角色、传输地址等。
当需要排查节点配置差异、确认插件安装状态、检查 JVM 堆内存配置是否合理时使用。不适用于节点运行时指标（请用 query-nodes-stats）。
参数 datasource 为数据源名称，可选；参数 node_id 指定节点 ID，可选，不指定则返回所有节点。
如需将节点 ID 翻译为 pod 名称，请使用 resolve-node。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"node_id":    {Type: "string", Description: "指定节点 ID（可选，不指定则返回所有节点）"},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.query-nodes-stats",
			Description: `获取节点运行时统计指标，包括 CPU 使用率、JVM 堆内存/GC 统计、磁盘 I/O、线程池队列/拒绝数、索引操作速率等。
当需要定位性能瓶颈、精确到单节点维度分析资源热点时使用。不适用于节点静态配置信息（请用 query-nodes-info）。
参数 datasource 为数据源名称，可选；参数 node_id 指定节点 ID，可选；参数 metric 为指标类型过滤，可选值：indices, os, process, jvm, thread_pool, fs, transport, http，不指定返回全部。
线程池拒绝（rejected > 0）是 ES 过载的重要信号，重点关注 write/search/bulk 线程池。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"node_id":    {Type: "string", Description: "指定节点 ID（可选）"},
					"metric":     {Type: "string", Description: "指标类型过滤：indices, os, process, jvm, thread_pool, fs, transport, http（可选，不指定返回全部）"},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.resolve-node",
			Description: `将节点 ID 翻译为可读的节点名称（即 k8s pod 名称）。如果节点已离线则返回当前所有在线节点列表供对比。
当告警或日志中出现节点 ID（如 9eTnKNwCQTOqO2DgKwAcJg）需要查明对应 pod 时使用。不适用于查询节点详细信息（请用 query-nodes-info）。
参数 node_id 为必填，为要查询的节点 ID；参数 datasource 为数据源名称，可选。
节点已离线时无法获取其名称，可通过 Rancher 查看 namespace 下最近重启的 pod 来确认。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"node_id":    {Type: "string", Description: "要查询的节点 ID（必填）"},
				},
				Required: []string{"node_id"},
			},
		},
		// ===== 索引操作 =====
		{
			Name: "foundation.elasticsearch.list-indices",
			Description: `列出集群中的索引，返回每个索引的健康状态、开关状态、文档数、存储大小、主分片/副本数等元数据。支持通配符模式匹配（如 logs-*）。
当需要进行日常索引管理、存储空间审计、索引生命周期监控时使用。不适用于查询单个索引详情（请用 query-index-detail）。
参数 datasource 为数据源名称，可选；参数 pattern 为索引名称模式，支持通配符 *，如 logs-2024*，可选。
大集群索引数量超过 200 时会自动生成摘要而非返回全量列表。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"pattern":    {Type: "string", Description: "索引名称模式，支持通配符 *（可选，如 logs-2024*）"},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.check-index-exists",
			Description: `检查指定索引是否存在于目标 Elasticsearch 集群中，返回布尔值。
当需要在执行索引操作前进行前置校验、索引创建前的幂等性判断时使用。不适用于获取索引详情（请用 query-index-detail）。
参数 index 为必填，为要检查的索引名称；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "要检查的索引名称"},
				},
				Required: []string{"index"},
			},
		},
		{
			Name: "foundation.elasticsearch.query-index-detail",
			Description: `获取索引的完整定义，包括 mappings（字段映射）、settings（分片/副本/分析器配置）、aliases（别名）。
当需要进行索引结构审查、迁移前的 schema 导出、排查字段类型冲突问题时使用。不适用于仅查看 mapping（请用 query-index-mapping）或 settings（请用 query-index-settings）。
参数 index 为必填，为索引名称；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称"},
				},
				Required: []string{"index"},
			},
		},
		{
			Name: "foundation.elasticsearch.query-index-mapping",
			Description: `获取索引的字段映射定义，包括字段名称、数据类型、分析器配置等。
当需要排查搜索不到数据的问题（字段类型不匹配）、确认动态映射是否生成了预期的字段类型、mapping 爆炸问题诊断时使用。不适用于查看索引设置（请用 query-index-settings）。
参数 index 为必填，为索引名称；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称"},
				},
				Required: []string{"index"},
			},
		},
		{
			Name: "foundation.elasticsearch.query-index-settings",
			Description: `获取索引的配置设置，包括主分片数、副本数、刷新间隔、分析器定义、translog 策略等。
当需要进行性能调优分析（如 refresh_interval 是否合理）、排查写入性能问题、确认索引配置是否符合规范时使用。不适用于查看字段映射（请用 query-index-mapping）。
参数 index 为必填，为索引名称；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称"},
				},
				Required: []string{"index"},
			},
		},
		{
			Name: "foundation.elasticsearch.query-index-stats",
			Description: `获取索引级别的详细统计信息，包括文档数、存储大小、索引/搜索/合并/刷新/flush 操作的次数和耗时。
当需要识别写入热点索引、搜索慢查询索引、段合并压力过大的索引时使用。不适用于集群级别统计（请用 query-cluster-stats）。
参数 datasource 为数据源名称，可选；参数 index 为索引名称，可选，不指定则返回所有索引的统计。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称（可选，不指定则返回所有索引的统计）"},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.create-index",
			Description: `创建新的 Elasticsearch 索引，可指定 mappings（字段映射）和 settings（分片数、副本数等配置）。
当需要按规范创建索引、初始化索引模板对应的索引时使用。不适用于修改已有索引（请用 Elasticsearch API 直接操作）。
参数 index 为必填，为索引名称；参数 settings 为索引设置，如 {"number_of_shards": 3, "number_of_replicas": 1}，可选；参数 mappings 为字段映射定义，可选。
创建前建议先调用 check-index-exists 确认索引不存在，避免重复创建报错。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称"},
					"settings":   {Type: "object", Description: "索引设置，如 {\"number_of_shards\": 3, \"number_of_replicas\": 1}（可选）"},
					"mappings":   {Type: "object", Description: "字段映射定义（可选）"},
				},
				Required: []string{"index"},
			},
		},
		{
			Name: "foundation.elasticsearch.open-index",
			Description: `打开已关闭的索引，使其恢复可读写状态。关闭的索引不占用集群资源但数据保留在磁盘上，打开后可重新提供服务。
当需要按需恢复访问历史索引数据时使用。不适用于关闭索引（请用 close-index）。
参数 index 为必填，为索引名称；参数 datasource 为数据源名称，可选。
打开大索引会触发分片恢复，可能短暂增加集群负载。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称"},
				},
				Required: []string{"index"},
			},
		},
		{
			Name: "foundation.elasticsearch.close-index",
			Description: `关闭索引，关闭后索引不可读写但数据保留在磁盘上，释放集群内存和计算资源。
当需要降低不再活跃但需保留的历史索引对集群的资源压力时使用。不适用于打开索引（请用 open-index）。
参数 index 为必填，为索引名称；参数 datasource 为数据源名称，可选。
关闭索引后数据不可查询，请确认业务不再需要后再操作。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称"},
				},
				Required: []string{"index"},
			},
		},
		// ===== 别名操作 =====
		{
			Name: "foundation.elasticsearch.list-aliases",
			Description: `列出索引别名信息，展示别名与索引的映射关系。
当需要排查别名指向错误、确认蓝绿部署切换状态、审计别名配置时使用。不适用于查询指定别名详情（请用 query-alias-detail）。
参数 datasource 为数据源名称，可选；参数 index 为按索引名称过滤，可选。
别名是 Elasticsearch 零停机索引切换的核心机制。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "按索引名称过滤（可选）"},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.query-alias-detail",
			Description: `获取指定别名的详细信息，包括该别名关联的所有索引、过滤条件、路由配置。
当需要确认别名的精确指向、排查查询路由问题时使用。不适用于列出所有别名（请用 list-aliases）。
参数 alias 为必填，为别名名称；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"alias":      {Type: "string", Description: "别名名称"},
				},
				Required: []string{"alias"},
			},
		},

		// ===== 文档操作 =====
		{
			Name: "foundation.elasticsearch.query-document",
			Description: `根据文档 ID 精确检索单个文档的完整内容，包括 _source 字段、版本号等元数据。
当需要进行数据验证、排查数据一致性问题、确认文档是否成功写入时使用。不适用于搜索多条文档（请用 search）或统计文档数（请用 count-documents）。
参数 index 为必填，为索引名称；参数 id 为必填，为文档 ID；参数 datasource 为数据源名称，可选。
文档不存在时会返回 [NOT_FOUND] 错误。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称"},
					"id":         {Type: "string", Description: "文档 ID"},
				},
				Required: []string{"index", "id"},
			},
		},
		{
			Name: "foundation.elasticsearch.count-documents",
			Description: `统计文档数量，支持按查询条件过滤。比 search 更轻量，不返回文档内容。
当需要进行数据完整性校验（如对比源端和目标端文档数）、监控索引增长趋势、验证数据清理效果时使用。不适用于获取文档内容（请用 query-document 或 search）。
参数 datasource 为数据源名称，可选；参数 index 为索引名称，可选，不指定则统计所有索引；参数 query 为查询条件（DSL 格式），可选，如 {"match": {"status": "error"}}。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称（可选，不指定则统计所有索引）"},
					"query":      {Type: "object", Description: "查询条件（DSL 格式，可选，如 {\"match\": {\"status\": \"error\"}}）"},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.create-document",
			Description: `索引（写入）一个文档到指定索引。可指定文档 ID 实现幂等写入，不指定则自动生成 ID。
当需要进行数据修复、测试数据注入、手动补录数据时使用。不适用于查询文档（请用 query-document）。
参数 index 为必填，为索引名称；参数 body 为必填，为文档内容（JSON 对象）；参数 id 为文档 ID，可选，不指定则自动生成；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称"},
					"id":         {Type: "string", Description: "文档 ID（可选，不指定则自动生成）"},
					"body":       {Type: "object", Description: "文档内容（JSON 对象）"},
				},
				Required: []string{"index", "body"},
			},
		},

		// ===== 搜索操作 =====
		{
			Name: "foundation.elasticsearch.search",
			Description: `执行 Elasticsearch DSL 搜索查询，支持全文检索、精确匹配、范围过滤、排序、分页、聚合分析、高亮显示等完整搜索能力。
当需要从 ES 索引中检索数据、分析日志、排查问题、执行聚合统计时使用。不适用于简单按 ID 获取文档（请用 query-document）或 SQL 风格查询（请用 query-sql）。
参数 query 为 Elasticsearch Query DSL 对象，如 {"match": {"message": "error"}}，可选（默认 match_all）；参数 index 为目标索引名称，可选（不指定则搜索所有索引）；参数 size 控制返回条数（默认 10），from 控制分页偏移（默认 0）；参数 sort 为排序规则，如 [{"@timestamp": "desc"}]；参数 _source 为源字段过滤；参数 aggs 为聚合定义；参数 highlight 为高亮配置；参数 track_total_hits 控制是否追踪总命中数。
建议始终指定 index 以避免全量搜索影响性能。from+size 总和不能超过 10000，深度翻页请使用 scroll-logs。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource":       {Type: "string", Description: datasourceDesc},
					"index":            {Type: "string", Description: "索引名称（可选，不指定则搜索所有索引）"},
					"query":            {Type: "object", Description: "查询 DSL，如 {\"match\": {\"message\": \"error\"}} 或 {\"bool\": {\"must\": [...]}}"},
					"size":             {Type: "integer", Description: "返回结果数量（默认 10）"},
					"from":             {Type: "integer", Description: "分页偏移量（默认 0）"},
					"sort": {
						Type:        "array",
						Description: "排序规则，如 [{\"@timestamp\": \"desc\"}]",
						Items:       &jsonschema.Schema{Type: "object"},
					},
					"_source": {
						Description: "源字段过滤：布尔值、字段名数组或 {includes/excludes} 对象",
						OneOf: []*jsonschema.Schema{
							{Type: "boolean"},
							{Type: "array", Items: &jsonschema.Schema{Type: "string"}},
							{Type: "object"},
						},
					},
					"aggs":             {Type: "object", Description: "聚合定义（可选），如 {\"status_count\": {\"terms\": {\"field\": \"status\"}}}"},
					"highlight":        {Type: "object", Description: "高亮配置（可选），如 {\"fields\": {\"message\": {}}}"},
					"track_total_hits": {Type: "boolean", Description: "是否追踪总命中数（默认 true，设为 false 可提升查询性能）"},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.query-sql",
			Description: `使用 SQL 语法查询 Elasticsearch 数据（需要 ES 支持 SQL 插件），支持 json/csv/txt 等返回格式。
当需要以更直观的 SQL 方式查询数据、快速数据探查和简单统计分析时使用。不适用于 DSL 搜索（请用 search）。
参数 query 为必填，为 SQL 查询语句，如 SELECT * FROM "logs-*" WHERE status = 500 LIMIT 10；参数 format 为返回格式，可选值：json（默认）、csv、txt；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"query":      {Type: "string", Description: "SQL 查询语句，如 SELECT * FROM \"logs-*\" WHERE status = 500 LIMIT 10"},
					"format":     {Type: "string", Description: "返回格式：json（默认）、csv、txt", Enum: []any{"json", "csv", "txt"}},
				},
				Required: []string{"query"},
			},
		},
		{
			Name: "foundation.elasticsearch.translate-sql",
			Description: `将 SQL 语句翻译为 Elasticsearch DSL 查询。
当需要学习 DSL 语法、将 SQL 思维转换为 ES 原生查询、优化查询性能（先用 SQL 表达意图，再基于翻译结果手动优化 DSL）时使用。不适用于执行 SQL 查询（请用 query-sql）。
参数 query 为必填，为 SQL 查询语句；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"query":      {Type: "string", Description: "SQL 查询语句"},
				},
				Required: []string{"query"},
			},
		},
		// ===== 分片与段 =====
		{
			Name: "foundation.elasticsearch.list-shards",
			Description: `获取分片级别的详细信息，包括分片状态（STARTED/UNASSIGNED/RELOCATING）、大小、文档数、所在节点。
当需要排查分片不均衡、未分配分片、数据倾斜问题时使用。不适用于段信息（请用 list-segments）。
参数 index 为必填，为索引名称，支持通配符如 logs-2024.01.*；参数 datasource 为数据源名称，可选。
大规模集群下必须指定 index 参数，不指定会返回全量分片数据（可能 20w+）导致超时。分片数超过 200 时会自动生成摘要。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称（必填，支持通配符如 logs-2024.01.*）"},
				},
				Required: []string{"index"},
			},
		},
		{
			Name: "foundation.elasticsearch.list-segments",
			Description: `获取 Lucene 段（segment）级别的信息，包括段大小、文档数、已删除文档数、是否可搜索等。
当需要诊断段合并（merge）问题、评估 force merge 的必要性、排查搜索性能下降（段数过多）时使用。不适用于分片信息（请用 list-shards）。
参数 index 为必填，为索引名称，支持通配符；参数 datasource 为数据源名称，可选。
大规模集群下必须指定 index 参数。段数超过 200 时会自动生成摘要。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称（必填，支持通配符如 logs-2024.01.*）"},
				},
				Required: []string{"index"},
			},
		},

		// ===== 任务管理 =====
		{
			Name: "foundation.elasticsearch.list-tasks",
			Description: `列出集群中正在运行的任务，包括搜索、索引、合并、快照等操作。
当需要排查长时间运行的任务（如卡住的 reindex）、监控批量操作进度、识别资源占用高的操作时使用。不适用于查询特定任务详情（请用 query-task-detail）。
参数 datasource 为数据源名称，可选；参数 detailed 为是否显示详细信息，可选（默认 false）；参数 actions 为按操作类型过滤，可选，如 indices:data/write/bulk。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"detailed":   {Type: "boolean", Description: "是否显示详细信息（默认 false）"},
					"actions":    {Type: "string", Description: "按操作类型过滤，如 indices:data/write/bulk（可选）"},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.query-task-detail",
			Description: `获取指定任务的详细信息和执行进度。任务 ID 格式为 node_id:task_number。
当需要跟踪异步操作（如 reindex、update_by_query）的完成进度和状态时使用。不适用于列出所有任务（请用 list-tasks）。
参数 task_id 为必填，格式为 node_id:task_number；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"task_id":    {Type: "string", Description: "任务 ID（格式：node_id:task_number）"},
				},
				Required: []string{"task_id"},
			},
		},

		// ===== 模板管理 =====
		{
			Name: "foundation.elasticsearch.list-templates",
			Description: `列出所有索引模板，显示模板名称、匹配模式、优先级等信息。
当需要审计模板配置、排查新索引配置不符合预期的问题时使用。不适用于查看模板详情（请用 query-template-detail）。
参数 datasource 为数据源名称，可选。
索引模板定义了新索引的默认 mappings 和 settings，是索引标准化管理的基础。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.query-template-detail",
			Description: `获取指定索引模板的完整定义，包括匹配模式、mappings、settings、aliases 配置。
当需要确认模板内容是否正确、排查新建索引的 mapping/settings 来源时使用。不适用于列出所有模板（请用 list-templates）。
参数 name 为必填，为模板名称；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"name":       {Type: "string", Description: "模板名称"},
				},
				Required: []string{"name"},
			},
		},

		// ===== 快照管理 =====
		{
			Name: "foundation.elasticsearch.list-repositories",
			Description: `列出所有已注册的快照仓库，包括仓库类型（fs/s3/hdfs 等）和配置信息。
当需要确认备份目标是否正确配置、排查快照失败的仓库连接问题时使用。不适用于列出快照（请用 list-snapshots）。
参数 datasource 为数据源名称，可选。
快照仓库是备份恢复的基础设施。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.list-snapshots",
			Description: `列出指定仓库中的所有快照，包括快照状态、包含的索引、开始/结束时间、分片成功/失败数。
当需要确认备份是否正常执行、查找可用于恢复的快照、排查快照失败原因时使用。不适用于列出仓库（请用 list-repositories）。
参数 repository 为必填，为快照仓库名称；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"repository": {Type: "string", Description: "快照仓库名称"},
				},
				Required: []string{"repository"},
			},
		},

		// ===== ILM 生命周期管理 =====
		{
			Name: "foundation.elasticsearch.list-ilm-policies",
			Description: `列出所有索引生命周期管理（ILM）策略，包括各阶段（hot/warm/cold/delete）的配置。
当需要审计策略配置、排查索引未按预期滚动/迁移/删除的问题时使用。不适用于查看索引 ILM 执行状态（请用 query-ilm-status）。
参数 datasource 为数据源名称，可选。
ILM 是自动化索引生命周期管理的核心机制。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.query-ilm-status",
			Description: `获取指定索引的 ILM 执行状态，包括当前所处阶段、步骤、执行时间、是否有错误等。
当需要排查索引 ILM 卡住（stuck）的问题、确认索引是否按预期在各阶段间流转时使用。不适用于列出 ILM 策略（请用 list-ilm-policies）。
参数 index 为必填，为索引名称；参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称"},
				},
				Required: []string{"index"},
			},
		},
		// ===== SRE 场景化诊断工具 =====
		{
			Name: "foundation.elasticsearch.list-allocation",
			Description: `获取各节点的分片分配和磁盘使用情况，包括分片数、已用磁盘、可用磁盘、磁盘使用百分比。
当集群出现磁盘水位线告警、分片无法分配、节点存储容量规划、分片分配不均衡时使用。不适用于线程池状态（请用 list-thread-pools）。
参数 datasource 为数据源名称，可选。
当集群出现分片无法分配时，首先检查此项。磁盘 >80% low watermark 停止分配，>85% high watermark 迁移分片，>95% flood stage 索引只读。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.list-thread-pools",
			Description: `获取各节点线程池的状态，包括活跃线程数、队列长度、拒绝次数。
当需要排查写入被拒绝（429 错误）、搜索超时等性能问题时使用。线程池拒绝（rejected > 0）是 ES 过载的重要信号，常见于 write/search/bulk 线程池。不适用于磁盘水位线（请用 list-allocation）。
参数 datasource 为数据源名称，可选；参数 thread_pool 为线程池名称过滤，如 write、search、bulk，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource":  {Type: "string", Description: datasourceDesc},
					"thread_pool": {Type: "string", Description: "线程池名称过滤，如 write、search、bulk（可选，不指定返回所有）"},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.list-pending-tasks",
			Description: `获取集群待处理的任务队列，包括任务优先级、来源、等待时间。
当需要排查集群响应变慢、索引创建/删除卡住等问题时使用。待处理任务堆积通常意味着 master 节点压力过大，可能导致集群状态更新延迟。不适用于运行中的任务（请用 list-tasks）。
参数 datasource 为数据源名称，可选。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
				},
			},
		},
		{
			Name: "foundation.elasticsearch.list-recovery",
			Description: `获取索引分片恢复进度信息，包括恢复类型（peer/snapshot/existing_store）、源/目标节点、已传输字节数、进度百分比。
当需要监控节点重启后的分片恢复进度、排查恢复缓慢的原因、评估集群恢复所需时间时使用。不适用于磁盘分配（请用 list-allocation）。
参数 index 为必填，为索引名称，支持通配符；参数 datasource 为数据源名称，可选。
大规模集群下必须指定 index 参数。恢复记录超过 100 条时会自动生成摘要。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称（必填，支持通配符如 logs-2024.01.*）"},
				},
				Required: []string{"index"},
			},
		},
		{
			Name: "foundation.elasticsearch.query-hot-threads",
			Description: `获取节点的热线程（Hot Threads）堆栈信息，显示 CPU 占用最高的线程正在执行的操作。
当需要排查节点 CPU 飙高问题、精确定位是搜索、索引、GC 还是合并操作导致的 CPU 瓶颈时使用。不适用于线程池状态（请用 list-thread-pools）。
参数 datasource 为数据源名称，可选；参数 node_id 为指定节点 ID，可选，不指定则返回所有节点。
这是排查节点 CPU 飙高的终极工具。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"node_id":    {Type: "string", Description: "指定节点 ID（可选，不指定则返回所有节点）"},
				},
			},
		},
	}
}

// ===== ElasticsearchTools 客户端辅助方法 =====

func (et *ElasticsearchTools) getClient(datasource string) (elasticsearch.Client, error) {
	if datasource == "" {
		datasource = et.defaultDatasource
	}
	// 精确匹配
	if client, exists := et.clients[datasource]; exists {
		return client, nil
	}
	// 模糊匹配：输入是某个 datasource 名称的前缀或子串时自动匹配
	var matched string
	for name := range et.clients {
		if strings.HasPrefix(name, datasource) || strings.Contains(name, datasource) {
			if matched != "" {
				return nil, fmt.Errorf("数据源 '%s' 不存在且模糊匹配到多个: %v，请指定完整名称", datasource, et.getAvailableDatasources())
			}
			matched = name
		}
	}
	if matched != "" {
		return et.clients[matched], nil
	}
	return nil, fmt.Errorf("数据源 '%s' 不存在，可用数据源: %v", datasource, et.getAvailableDatasources())
}

func (et *ElasticsearchTools) getClientWithConnectionCheck(ctx context.Context, datasource string) (elasticsearch.Client, error) {
	client, err := et.getClient(datasource)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx); err != nil {
		return nil, fmt.Errorf("数据源 '%s' 连接不可用: %v。可用数据源: %v", datasource, err, et.getAvailableDatasources())
	}
	return client, nil
}

func (et *ElasticsearchTools) getAvailableDatasources() []string {
	names := make([]string, 0, len(et.clients))
	for name := range et.clients {
		names = append(names, name)
	}
	return names
}

// HandleTool 处理 MCP 工具调用并将其路由到适当的处理程序
func (et *ElasticsearchTools) HandleTool(ctx context.Context, toolName string, arguments map[string]interface{}) mcp.CallToolResult {
	// 先检查是否是 Discover 工具
	if result, handled := et.HandleDiscoverTool(ctx, toolName, arguments); handled {
		return result
	}

	switch toolName {
	// 实例与集群
	case "foundation.elasticsearch.list-instances":
		return et.handleListInstances(ctx, arguments)
	case "foundation.elasticsearch.query-cluster-info":
		return et.handleClusterInfo(ctx, arguments)
	case "foundation.elasticsearch.query-cluster-health":
		return et.handleClusterHealth(ctx, arguments)
	case "foundation.elasticsearch.analyze-cluster-health":
		return et.handleClusterHealthAnalysis(ctx, arguments)
	case "foundation.elasticsearch.query-cluster-stats":
		return et.handleClusterStats(ctx, arguments)
	case "foundation.elasticsearch.query-nodes-info":
		return et.handleNodesInfo(ctx, arguments)
	case "foundation.elasticsearch.query-nodes-stats":
		return et.handleNodesStats(ctx, arguments)
	case "foundation.elasticsearch.resolve-node":
		return et.handleResolveNode(ctx, arguments)
	// 索引操作
	case "foundation.elasticsearch.list-indices":
		return et.handleIndexList(ctx, arguments)
	case "foundation.elasticsearch.check-index-exists":
		return et.handleIndexExists(ctx, arguments)
	case "foundation.elasticsearch.query-index-detail":
		return et.handleGetIndex(ctx, arguments)
	case "foundation.elasticsearch.query-index-mapping":
		return et.handleGetMapping(ctx, arguments)
	case "foundation.elasticsearch.query-index-settings":
		return et.handleGetSettings(ctx, arguments)
	case "foundation.elasticsearch.query-index-stats":
		return et.handleIndexStats(ctx, arguments)
	case "foundation.elasticsearch.create-index":
		return et.handleIndexCreate(ctx, arguments)
	case "foundation.elasticsearch.open-index":
		return et.handleOpenIndex(ctx, arguments)
	case "foundation.elasticsearch.close-index":
		return et.handleCloseIndex(ctx, arguments)
	// 别名
	case "foundation.elasticsearch.list-aliases":
		return et.handleListAliases(ctx, arguments)
	case "foundation.elasticsearch.query-alias-detail":
		return et.handleGetAlias(ctx, arguments)
	// 文档
	case "foundation.elasticsearch.query-document":
		return et.handleDocumentGet(ctx, arguments)
	case "foundation.elasticsearch.count-documents":
		return et.handleDocumentCount(ctx, arguments)
	case "foundation.elasticsearch.create-document":
		return et.handleDocumentIndex(ctx, arguments)
	// 搜索
	case "foundation.elasticsearch.search":
		return et.handleSearch(ctx, arguments)
	case "foundation.elasticsearch.query-sql":
		return et.handleSQLQuery(ctx, arguments)
	case "foundation.elasticsearch.translate-sql":
		return et.handleSQLTranslate(ctx, arguments)
	// 分片与段
	case "foundation.elasticsearch.list-shards":
		return et.handleCatShards(ctx, arguments)
	case "foundation.elasticsearch.list-segments":
		return et.handleCatSegments(ctx, arguments)
	// 任务
	case "foundation.elasticsearch.list-tasks":
		return et.handleListTasks(ctx, arguments)
	case "foundation.elasticsearch.query-task-detail":
		return et.handleGetTask(ctx, arguments)
	// 模板
	case "foundation.elasticsearch.list-templates":
		return et.handleListTemplates(ctx, arguments)
	case "foundation.elasticsearch.query-template-detail":
		return et.handleGetTemplate(ctx, arguments)
	// 快照
	case "foundation.elasticsearch.list-repositories":
		return et.handleListRepositories(ctx, arguments)
	case "foundation.elasticsearch.list-snapshots":
		return et.handleListSnapshots(ctx, arguments)
	// ILM
	case "foundation.elasticsearch.list-ilm-policies":
		return et.handleListILMPolicies(ctx, arguments)
	case "foundation.elasticsearch.query-ilm-status":
		return et.handleGetILMStatus(ctx, arguments)
	// SRE 诊断
	case "foundation.elasticsearch.list-allocation":
		return et.handleCatAllocation(ctx, arguments)
	case "foundation.elasticsearch.list-thread-pools":
		return et.handleCatThreadPool(ctx, arguments)
	case "foundation.elasticsearch.list-pending-tasks":
		return et.handleCatPendingTasks(ctx, arguments)
	case "foundation.elasticsearch.list-recovery":
		return et.handleCatRecovery(ctx, arguments)
	case "foundation.elasticsearch.query-hot-threads":
		return et.handleNodesHotThreads(ctx, arguments)
	default:
		return createErrorResult(fmt.Sprintf("未知工具: %s", toolName))
	}
}
