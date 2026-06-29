// Package tools - discover.go 提供面向开发者的日志查询与排查工具。
// 核心场景：开发通过 MCP 工具查询特定时间段的日志，进行 troubleshooting。
// 设计原则：
//   1. 控制返回数据量，避免超出大模型上下文窗口
//   2. 长日志消息自动截断，保留关键信息
//   3. 提供聚合摘要工具，用统计代替原始日志
//   4. 支持 search_after 深度翻页，按需获取更多
package tools

import (
	"context"
	"fmt"
	"strings"

	"elasticsearch-mcp/elasticsearch"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 日志截断相关常量
const (
	defaultMaxFieldLen  = 500   // 单个字段默认最大字符数
	absoluteMaxFieldLen = 5000  // 用户可设置的最大值上限
	defaultLogSize      = 20    // 默认返回日志条数
	maxLogSize          = 200   // 最大返回日志条数
	defaultContextCount = 5     // 上下文默认前后条数
	maxResponseChars    = 80000 // 单次响应最大字符数估算（留安全余量给大模型上下文）
)

// ===== Discover 工具定义 =====

// GetDiscoverTools 返回所有日志查询工具定义
func (et *ElasticsearchTools) GetDiscoverTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name: "foundation.elasticsearch.discover-logs",
			Description: `面向开发者的日志搜索工具，类似 Kibana Discover。指定索引、时间范围、关键词，返回匹配的日志条目。内置上下文保护机制：默认只返回关键字段、字符串字段自动截断（500字符）、默认 20 条最多 200 条、支持 compact 紧凑模式、数据量过大时自动降级。
当需要进行应用日志排查、错误日志检索、关键词搜索定位问题时使用。不适用于聚合统计（请用 summarize-logs）或深度翻页（请用 scroll-logs）。
参数 index 为必填，为索引名称或模式；参数 keyword 为搜索关键词，支持 Lucene 语法；参数 start_time/end_time 为时间范围；参数 size 控制返回条数；参数 filter 为额外 DSL 过滤条件；参数 _source 为返回字段过滤；参数 max_field_len 为字段截断长度；参数 compact 为紧凑模式；参数 track_total_hits 控制是否追踪总命中数。
如索引不存在会返回 [NOT_FOUND]，集群连接失败会返回 [DATASOURCE_ERROR]，查询语法错误会返回 [ES_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource":        {Type: "string", Description: datasourceDesc},
					"index":             {Type: "string", Description: "索引名称或模式，支持通配符（如 app-logs-*、bff-user-2024.01.*）"},
					"keyword":           {Type: "string", Description: "搜索关键词，支持 Lucene 语法（如 error AND timeout、level:ERROR）。为空则匹配所有"},
					"time_field":        {Type: "string", Description: "时间字段名（默认 @timestamp）"},
					"start_time":        {Type: "string", Description: "开始时间，支持相对时间（now-1h、now-15m）或绝对时间（2024-01-01T00:00:00Z、2024-01-01 00:00:00）"},
					"end_time":          {Type: "string", Description: "结束时间，格式同 start_time（默认 now）"},
					"size":              {Type: "integer", Description: "返回日志条数（默认 20，最大 200）"},
					"sort_order":        {Type: "string", Description: "时间排序：desc（默认，最新在前）或 asc", Enum: []any{"desc", "asc"}},
					"filter":            {Type: "object", Description: "额外 DSL 过滤条件（如 {\"term\": {\"appName\": \"menu-shell\"}}）"},
					"_source":           {Description: "返回字段过滤：字段名数组如 [\"message\",\"@timestamp\",\"level\"]。不传则自动选择关键字段"},
					"max_field_len":     {Type: "integer", Description: "单个字段最大字符数（默认 500，最大 5000，设为 0 不截断）"},
					"compact":           {Type: "boolean", Description: "紧凑模式：true 时每条日志压缩为一行摘要（时间|级别|消息前200字符），极端节省上下文"},
					"track_total_hits":  {Type: "boolean", Description: "是否追踪总命中数（默认 true，设为 false 可提升查询性能）"},
				},
				Required: []string{"index"},
			},
		},

		{
			Name: "foundation.elasticsearch.summarize-logs",
			Description: `对大量日志做服务端聚合统计，不返回原始日志，只返回统计结论。返回内容包括：总日志数、按 level 分布统计、错误消息 Top N、按指定字段分组统计、时间段内日志量趋势。适用于日志量大时的 troubleshooting 第一步：先看全貌，再针对性查询。
当需要了解日志全貌、快速定位错误分布、排查哪个 pod/service 错误最多时使用。不适用于查看原始日志（请用 discover-logs）。
参数 index 为必填，为索引名称或模式；参数 keyword 为搜索关键词；参数 start_time/end_time 为时间范围；参数 filter 为额外 DSL 过滤条件；参数 group_by_fields 为分组统计字段数组；参数 message_field/level_field 可自定义字段名；参数 top_n 控制 Top N 数量。
如索引不存在会返回 [NOT_FOUND]，集群连接失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource":      {Type: "string", Description: datasourceDesc},
					"index":           {Type: "string", Description: "索引名称或模式"},
					"keyword":         {Type: "string", Description: "搜索关键词（可选）"},
					"time_field":      {Type: "string", Description: "时间字段名（默认 @timestamp）"},
					"start_time":      {Type: "string", Description: "开始时间，支持相对时间（now-1h、now-15m）或绝对时间（2024-01-01T00:00:00Z）"},
					"end_time":        {Type: "string", Description: "结束时间，格式同 start_time（默认 now）"},
					"filter":          {Type: "object", Description: "额外 DSL 过滤条件（可选）"},
					"group_by_fields": {Type: "array", Description: "按哪些字段分组统计（可选，如 [\"kubernetes.pod_name\",\"level\"]）", Items: &jsonschema.Schema{Type: "string"}},
					"message_field":   {Type: "string", Description: "日志消息字段名（默认 message）"},
					"level_field":     {Type: "string", Description: "日志级别字段名（默认 level）"},
					"top_n":           {Type: "integer", Description: "错误消息和分组的 Top N 数量（默认 10）"},
				},
				Required: []string{"index"},
			},
		},

		{
			Name: "foundation.elasticsearch.query-log-context",
			Description: `查看某条日志前后的上下文，类似 Kibana 的 "View surrounding documents"。提供一条日志的时间戳，获取该时间点前后各 N 条日志，帮助理解问题发生的完整上下文。同样有字段截断保护。
当需要理解错误发生前后的完整链路、追踪请求上下文、查看异常前后关联日志时使用。不适用于搜索日志（请用 discover-logs）或聚合统计（请用 summarize-logs）。
参数 index 为必填，为索引名称；参数 timestamp 为必填，为目标日志的时间戳（ISO8601）；参数 doc_id 为目标日志 ID（可选，精确定位）；参数 before_count/after_count 为前后条数（默认各 5）；参数 filter 为额外过滤条件；参数 _source 为返回字段过滤；参数 max_field_len 为字段截断长度。
如索引不存在会返回 [NOT_FOUND]，集群连接失败会返回 [DATASOURCE_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource":    {Type: "string", Description: datasourceDesc},
					"index":         {Type: "string", Description: "索引名称"},
					"timestamp":     {Type: "string", Description: "目标日志的时间戳（ISO8601，如 2024-01-15T10:30:00.123Z）"},
					"time_field":    {Type: "string", Description: "时间字段名（默认 @timestamp）"},
					"doc_id":        {Type: "string", Description: "目标日志的文档 ID（可选，用于精确定位）"},
					"before_count":  {Type: "integer", Description: "向前（更早）获取的条数（默认 5）"},
					"after_count":   {Type: "integer", Description: "向后（更晚）获取的条数（默认 5）"},
					"filter":        {Type: "object", Description: "额外过滤条件（如限定同一 pod）"},
					"_source":       {Description: "返回字段过滤（可选）"},
					"max_field_len": {Type: "integer", Description: "字段最大字符数（默认 500）"},
				},
				Required: []string{"index", "timestamp"},
			},
		},

		{
			Name: "foundation.elasticsearch.query-field-stats",
			Description: `获取索引的字段列表、类型和值分布，帮助构建查询条件。返回字段名称、数据类型、是否可搜索/可聚合，以及 keyword/boolean 字段的 Top N 值分布和数值字段的 min/max/avg。
当不确定索引有哪些字段、字段叫什么名字、字段值有哪些时使用。不适用于查询日志（请用 discover-logs）或搜索文档（请用 search）。
参数 index 为必填，为索引名称或模式；参数 fields 为指定要统计的字段数组（可选）；参数 time_field/start_time/end_time 为统计时间范围；参数 top_n 为每个字段的 Top N 值数量（默认 10）。
如索引不存在会返回 [NOT_FOUND]，集群连接失败会返回 [DATASOURCE_ERROR]，字段信息获取失败会返回 [ES_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource": {Type: "string", Description: datasourceDesc},
					"index":      {Type: "string", Description: "索引名称或模式"},
					"time_field": {Type: "string", Description: "时间字段名（默认 @timestamp）"},
					"start_time": {Type: "string", Description: "统计时间范围起点，支持相对时间（now-1h）或绝对时间（2024-01-01T00:00:00Z），可选"},
					"end_time":   {Type: "string", Description: "统计时间范围终点，格式同 start_time，可选"},
					"fields":     {Type: "array", Description: "指定要统计的字段（可选，不指定则自动选择）", Items: &jsonschema.Schema{Type: "string"}},
					"top_n":      {Type: "integer", Description: "每个字段的 Top N 值数量（默认 10）"},
				},
				Required: []string{"index"},
			},
		},

		{
			Name: "foundation.elasticsearch.scroll-logs",
			Description: `基于 search_after 的深度翻页，突破 from+size 的 10000 条限制。首次不传 search_after，后续传入上一页返回的 next_search_after 值翻到下一页。同样有字段截断保护，每次只返回一页数据。
当需要遍历大量日志、导出全量匹配结果、深度翻页超过 10000 条限制时使用。不适用于常规日志查询（请用 discover-logs）。
参数 index 为必填，为索引名称或模式；参数 search_after 为上一页返回的翻页游标（首次不传）；参数 keyword 为搜索关键词；参数 start_time/end_time 为时间范围；参数 size 为每页条数；参数 filter 为额外 DSL 过滤条件；参数 _source 为返回字段过滤；参数 max_field_len 为字段截断长度；参数 compact 为紧凑模式。
如索引不存在会返回 [NOT_FOUND]，集群连接失败会返回 [DATASOURCE_ERROR]，查询语法错误会返回 [ES_ERROR]。`,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"datasource":    {Type: "string", Description: datasourceDesc},
					"index":         {Type: "string", Description: "索引名称或模式"},
					"keyword":       {Type: "string", Description: "搜索关键词（可选）"},
					"time_field":    {Type: "string", Description: "时间字段名（默认 @timestamp）"},
					"start_time":    {Type: "string", Description: "开始时间，支持相对时间（now-1h）或绝对时间（2024-01-01T00:00:00Z），可选"},
					"end_time":      {Type: "string", Description: "结束时间，格式同 start_time，可选"},
					"size":          {Type: "integer", Description: "每页条数（默认 20，最大 200）"},
					"sort_order":    {Type: "string", Description: "排序：desc（默认）或 asc", Enum: []any{"desc", "asc"}},
					"filter":        {Type: "object", Description: "额外 DSL 过滤条件（可选）"},
					"_source":       {Description: "返回字段过滤（可选）"},
					"search_after":  {Type: "array", Description: "上一页返回的 next_search_after 值（首次不传）", Items: &jsonschema.Schema{}},
					"max_field_len": {Type: "integer", Description: "字段最大字符数（默认 500）"},
					"compact":       {Type: "boolean", Description: "紧凑模式（默认 false）"},
				},
				Required: []string{"index"},
			},
		},
	}
}

// ===== 路由 =====

// HandleDiscoverTool 路由 Discover 工具调用
func (et *ElasticsearchTools) HandleDiscoverTool(ctx context.Context, toolName string, arguments map[string]interface{}) (mcp.CallToolResult, bool) {
	switch toolName {
	case "foundation.elasticsearch.discover-logs":
		return et.handleDiscover(ctx, arguments), true
	case "foundation.elasticsearch.summarize-logs":
		return et.handleLogSummary(ctx, arguments), true
	case "foundation.elasticsearch.query-log-context":
		return et.handleLogContext(ctx, arguments), true
	case "foundation.elasticsearch.query-field-stats":
		return et.handleFieldStats(ctx, arguments), true
	case "foundation.elasticsearch.scroll-logs":
		return et.handleScrollLogs(ctx, arguments), true
	default:
		return mcp.CallToolResult{}, false
	}
}

// ===== 辅助函数 =====

// buildTimeRangeFilter 构建时间范围过滤
func buildTimeRangeFilter(timeField, startTime, endTime string) map[string]interface{} {
	if timeField == "" {
		timeField = "@timestamp"
	}
	rangeQ := map[string]interface{}{
		"format": "strict_date_optional_time||epoch_millis",
	}
	if startTime != "" {
		rangeQ["gte"] = startTime
	}
	if endTime != "" {
		rangeQ["lte"] = endTime
	} else {
		rangeQ["lte"] = "now"
	}
	return map[string]interface{}{
		"range": map[string]interface{}{timeField: rangeQ},
	}
}

// buildQueryFromKeyword 根据关键词构建查询
func buildQueryFromKeyword(keyword string) map[string]interface{} {
	if keyword == "" {
		return map[string]interface{}{"match_all": map[string]interface{}{}}
	}
	return map[string]interface{}{
		"query_string": map[string]interface{}{
			"query":            keyword,
			"analyze_wildcard": true,
			"default_operator": "AND",
		},
	}
}

// buildBoolQuery 组合 must + filter
func buildBoolQuery(query map[string]interface{}, filters ...map[string]interface{}) map[string]interface{} {
	filterList := make([]interface{}, 0)
	for _, f := range filters {
		if f != nil {
			filterList = append(filterList, f)
		}
	}
	boolQ := map[string]interface{}{"must": []interface{}{query}}
	if len(filterList) > 0 {
		boolQ["filter"] = filterList
	}
	return map[string]interface{}{"bool": boolQ}
}

// getIntArg 提取整数参数
func getIntArg(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return defaultVal
}

// truncateMessage 截断字符串字段，保留头尾信息
func truncateMessage(msg string, maxLen int) string {
	if maxLen <= 0 || len(msg) <= maxLen {
		return msg
	}
	// 保留前 70% 和后 20%，中间用省略标记
	headLen := maxLen * 7 / 10
	tailLen := maxLen * 2 / 10
	omitted := len(msg) - maxLen
	return fmt.Sprintf("%s\n... [省略 %d 字符] ...\n%s", msg[:headLen], omitted, msg[len(msg)-tailLen:])
}

// truncateHits 对搜索结果中的日志做字段截断（所有字符串字段）
func truncateHits(hits []interface{}, maxFieldLen int, msgFields []string) []interface{} {
	if maxFieldLen <= 0 {
		return hits
	}
	for _, hit := range hits {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}
		source, ok := hitMap["_source"].(map[string]interface{})
		if !ok {
			continue
		}
		truncateMapFields(source, maxFieldLen)
	}
	return hits
}

// truncateMapFields 递归截断 map 中所有字符串字段
func truncateMapFields(m map[string]interface{}, maxLen int) {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			m[k] = truncateMessage(val, maxLen)
		case map[string]interface{}:
			truncateMapFields(val, maxLen)
		}
	}
}

// compactHits 将 hits 压缩为一行摘要格式
func compactHits(hits []interface{}, timeField string) []string {
	lines := make([]string, 0, len(hits))
	for _, hit := range hits {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}
		source, ok := hitMap["_source"].(map[string]interface{})
		if !ok {
			continue
		}
		ts := extractStringField(source, timeField, "@timestamp")
		level := extractStringField(source, "level", "logLevel", "log_level", "severity")
		msg := extractStringField(source, "message", "msg", "log", "ext.msg", "content")
		app := extractStringField(source, "appName", "app", "service", "kubernetes.container_name")
		pod := extractStringField(source, "ext.pod", "kubernetes.pod_name", "pod_name", "pod")

		// 截断消息到 200 字符
		if len(msg) > 200 {
			msg = msg[:200] + "..."
		}

		line := fmt.Sprintf("[%s] %s", ts, level)
		if app != "" {
			line += " " + app
		}
		if pod != "" {
			line += " (" + pod + ")"
		}
		line += " | " + msg
		lines = append(lines, line)
	}
	return lines
}

// extractStringField 从 source 中提取第一个存在的字符串字段
func extractStringField(source map[string]interface{}, candidates ...string) string {
	for _, field := range candidates {
		// 支持嵌套字段如 "ext.msg"
		if strings.Contains(field, ".") {
			parts := strings.SplitN(field, ".", 2)
			if nested, ok := source[parts[0]].(map[string]interface{}); ok {
				if val, ok := nested[parts[1]].(string); ok && val != "" {
					return val
				}
			}
		}
		if val, ok := source[field].(string); ok && val != "" {
			return val
		}
	}
	return ""
}

// defaultSourceFields 默认只返回的关键字段列表
var defaultSourceFields = []string{
	"@timestamp", "timestamp",
	"level", "logLevel", "log_level", "severity",
	"message", "msg", "log",
	"appName", "app", "service",
	"ext.msg", "ext.pod", "ext.thread", "ext.time", "ext.taken",
	"kubernetes.pod_name", "kubernetes.namespace", "kubernetes.container_name",
	"error_message", "stack_trace", "exception",
	"status", "status_code", "http_status",
	"request_uri", "request_url", "path",
	"trace_id", "traceId", "span_id",
}

// extractHits 从搜索响应中提取 hits 数组
func extractHits(response map[string]interface{}) []interface{} {
	if hits, ok := response["hits"].(map[string]interface{}); ok {
		if hitArr, ok := hits["hits"].([]interface{}); ok {
			return hitArr
		}
	}
	return []interface{}{}
}

// extractTotalHits 从搜索响应中提取总命中数
func extractTotalHits(response map[string]interface{}) int64 {
	if hits, ok := response["hits"].(map[string]interface{}); ok {
		if total, ok := hits["total"].(map[string]interface{}); ok {
			if val, ok := total["value"].(float64); ok {
				return int64(val)
			}
		}
	}
	return 0
}

// reverseHits 反转 hits 数组
func reverseHits(hits []interface{}) {
	for i, j := 0, len(hits)-1; i < j; i, j = i+1, j-1 {
		hits[i], hits[j] = hits[j], hits[i]
	}
}

// buildSortClause 构建排序子句
func buildSortClause(timeField, sortOrder string) []interface{} {
	return []interface{}{
		map[string]interface{}{timeField: map[string]interface{}{"order": sortOrder, "unmapped_type": "date"}},
		map[string]interface{}{"_id": map[string]interface{}{"order": sortOrder}},
	}
}

// resolveMaxFieldLen 解析 max_field_len 参数
func resolveMaxFieldLen(args map[string]interface{}) int {
	v := getIntArg(args, "max_field_len", defaultMaxFieldLen)
	if v < 0 {
		return 0
	}
	if v > absoluteMaxFieldLen {
		return absoluteMaxFieldLen
	}
	return v
}

// isCompactMode 检查是否启用紧凑模式
func isCompactMode(args map[string]interface{}) bool {
	if v, ok := args["compact"]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// estimateHitsSize 估算 hits JSON 大小（字符数）
func estimateHitsSize(hits []interface{}) int {
	total := 0
	for _, hit := range hits {
		if hitMap, ok := hit.(map[string]interface{}); ok {
			if source, ok := hitMap["_source"].(map[string]interface{}); ok {
				for _, v := range source {
					if s, ok := v.(string); ok {
						total += len(s)
					} else {
						total += 20 // 非字符串字段估算
					}
				}
			}
		}
		total += 100 // 元数据开销
	}
	return total
}

// ===== handleDiscover =====

func (et *ElasticsearchTools) handleDiscover(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource := getArg(args, "datasource")
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, "连接不可用", et.getAvailableDatasources())
	}

	index := getArg(args, "index")
	if index == "" {
		return createParamError("index", "为必填项")
	}

	keyword := getArg(args, "keyword")
	timeField := getArg(args, "time_field")
	if timeField == "" {
		timeField = "@timestamp"
	}
	startTime := getArg(args, "start_time")
	endTime := getArg(args, "end_time")
	sortOrder := getArg(args, "sort_order")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	size := getIntArg(args, "size", defaultLogSize)
	if size > maxLogSize {
		size = maxLogSize
	}
	maxFieldLen := resolveMaxFieldLen(args)
	compact := isCompactMode(args)

	// 解析 track_total_hits 参数，默认为 true
	trackTotalHits := true
	if v, ok := args["track_total_hits"]; ok {
		if b, ok := v.(bool); ok {
			trackTotalHits = b
		}
	}

	// 构建查询
	query := buildQueryFromKeyword(keyword)
	var filters []map[string]interface{}
	if startTime != "" || endTime != "" {
		filters = append(filters, buildTimeRangeFilter(timeField, startTime, endTime))
	}
	if f, ok := args["filter"].(map[string]interface{}); ok {
		filters = append(filters, f)
	}
	finalQuery := buildBoolQuery(query, filters...)

	// 智能 _source 选择：用户没传 _source 时，默认只返回关键字段
	searchReq := &elasticsearch.SearchRequestExtended{
		SearchRequest: elasticsearch.SearchRequest{
			Index:          index,
			Query:          finalQuery,
			Size:           size,
			Sort:           buildSortClause(timeField, sortOrder),
			TrackTotalHits: &trackTotalHits,
		},
	}
	if src, ok := args["_source"]; ok {
		searchReq.Source = src
	} else {
		// 默认只返回关键字段，大幅减少返回数据量
		searchReq.Source = defaultSourceFields
	}

	result, err := client.SearchWithAggs(ctx, searchReq)
	if err != nil {
		return createESError("日志查询", err.Error())
	}

	totalHits := extractTotalHits(*result)
	hits := extractHits(*result)

	// 截断所有字符串字段
	hits = truncateHits(hits, maxFieldLen, nil)

	// 估算大小，如果太大自动降级
	estimatedSize := estimateHitsSize(hits)
	autoCompacted := false
	if !compact && estimatedSize > maxResponseChars {
		compact = true
		autoCompacted = true
	}

	// 提取翻页游标
	var nextSearchAfter interface{}
	if len(hits) > 0 {
		if lastHit, ok := hits[len(hits)-1].(map[string]interface{}); ok {
			nextSearchAfter = lastHit["sort"]
		}
	}
	hasMore := int64(len(hits)) < totalHits

	// 构建输出
	output := map[string]interface{}{
		"total_hits":        totalHits,
		"returned_count":    len(hits),
		"has_more":          hasMore,
		"next_search_after": nextSearchAfter,
		"query_info": map[string]interface{}{
			"index": index, "keyword": keyword, "time_field": timeField,
			"start_time": startTime, "end_time": endTime, "sort_order": sortOrder,
		},
	}

	if compact {
		output["compact_logs"] = compactHits(hits, timeField)
		output["mode"] = "compact"
	} else {
		output["hits"] = hits
		output["mode"] = "full"
	}

	// 摘要
	summary := fmt.Sprintf("索引 '%s' 查询完成：共命中 %d 条，返回 %d 条", index, totalHits, len(hits))
	if autoCompacted {
		summary += "（数据量较大，已自动切换为紧凑模式）"
	} else if compact {
		summary += "（紧凑模式）"
	}
	if maxFieldLen > 0 {
		summary += fmt.Sprintf("，字段截断至 %d 字符", maxFieldLen)
	}
	if hasMore {
		summary += "。还有更多结果，可用 next_search_after 翻页或用 summarize-logs 查看聚合统计"
	}
	return createSuccessResult(summary, output)
}

// ===== handleLogSummary =====

func (et *ElasticsearchTools) handleLogSummary(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource := getArg(args, "datasource")
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, "连接不可用", et.getAvailableDatasources())
	}

	index := getArg(args, "index")
	if index == "" {
		return createParamError("index", "为必填项")
	}

	keyword := getArg(args, "keyword")
	timeField := getArg(args, "time_field")
	if timeField == "" {
		timeField = "@timestamp"
	}
	startTime := getArg(args, "start_time")
	endTime := getArg(args, "end_time")
	messageField := getArg(args, "message_field")
	if messageField == "" {
		messageField = "message"
	}
	levelField := getArg(args, "level_field")
	if levelField == "" {
		levelField = "level"
	}
	topN := getIntArg(args, "top_n", 10)

	// 构建查询
	query := buildQueryFromKeyword(keyword)
	var filters []map[string]interface{}
	if startTime != "" || endTime != "" {
		filters = append(filters, buildTimeRangeFilter(timeField, startTime, endTime))
	}
	if f, ok := args["filter"].(map[string]interface{}); ok {
		filters = append(filters, f)
	}
	finalQuery := buildBoolQuery(query, filters...)

	// 构建聚合：level 分布 + 消息关键词 Top N + 自定义分组 + 时间趋势
	aggs := map[string]interface{}{
		// 按日志级别分布
		"level_distribution": map[string]interface{}{
			"terms": map[string]interface{}{
				"field": levelField + ".keyword",
				"size":  20,
			},
		},
		// 错误消息 Top N（对 message 字段做 significant_terms 或 terms）
		// 使用 message.keyword 的 terms 聚合（如果存在）
		"top_messages": map[string]interface{}{
			"terms": map[string]interface{}{
				"field": messageField + ".keyword",
				"size":  topN,
			},
		},
		// 时间趋势（粗粒度，固定 20 个桶）
		"time_trend": map[string]interface{}{
			"auto_date_histogram": map[string]interface{}{
				"field":   timeField,
				"buckets": 20,
			},
		},
	}

	// 自定义分组字段
	if groupFields, ok := args["group_by_fields"].([]interface{}); ok {
		for _, gf := range groupFields {
			if fieldName, ok := gf.(string); ok && fieldName != "" {
				safeKey := "group_" + strings.ReplaceAll(fieldName, ".", "_")
				aggs[safeKey] = map[string]interface{}{
					"terms": map[string]interface{}{
						"field": fieldName,
						"size":  topN,
					},
				}
			}
		}
	}

	// 额外：按 level=ERROR 过滤后的 message Top N
	aggs["error_messages"] = map[string]interface{}{
		"filter": map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []interface{}{
					map[string]interface{}{"term": map[string]interface{}{levelField + ".keyword": "ERROR"}},
					map[string]interface{}{"term": map[string]interface{}{levelField + ".keyword": "error"}},
					map[string]interface{}{"term": map[string]interface{}{levelField + ".keyword": "Error"}},
				},
				"minimum_should_match": 1,
			},
		},
		"aggs": map[string]interface{}{
			"messages": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": messageField + ".keyword",
					"size":  topN,
				},
			},
		},
	}

	searchReq := &elasticsearch.SearchRequestExtended{
		SearchRequest: elasticsearch.SearchRequest{
			Index: index,
			Query: finalQuery,
			Size:  0, // 不返回原始日志
		},
		Aggs: aggs,
	}

	result, err := client.SearchWithAggs(ctx, searchReq)
	if err != nil {
		return createESError("日志聚合查询", err.Error())
	}

	totalHits := extractTotalHits(*result)

	// 提取聚合结果
	aggregations, _ := (*result)["aggregations"].(map[string]interface{})

	output := map[string]interface{}{
		"total_hits": totalHits,
		"query_info": map[string]interface{}{
			"index":      index,
			"keyword":    keyword,
			"start_time": startTime,
			"end_time":   endTime,
		},
		"aggregations": aggregations,
	}

	// 构建可读摘要
	summary := fmt.Sprintf("索引 '%s' 日志聚合摘要：共 %d 条日志", index, totalHits)

	// 提取 level 分布信息
	if levelDist, ok := aggregations["level_distribution"].(map[string]interface{}); ok {
		if buckets, ok := levelDist["buckets"].([]interface{}); ok {
			levels := []string{}
			for _, b := range buckets {
				if bMap, ok := b.(map[string]interface{}); ok {
					key, _ := bMap["key"].(string)
					count, _ := bMap["doc_count"].(float64)
					levels = append(levels, fmt.Sprintf("%s:%d", key, int64(count)))
				}
			}
			if len(levels) > 0 {
				summary += fmt.Sprintf("，级别分布: %s", strings.Join(levels, " / "))
			}
		}
	}

	summary += "。不返回原始日志，如需查看具体日志请使用 discover-logs"
	return createSuccessResult(summary, output)
}

// ===== handleLogContext =====

func (et *ElasticsearchTools) handleLogContext(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource := getArg(args, "datasource")
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, "连接不可用", et.getAvailableDatasources())
	}

	index := getArg(args, "index")
	if index == "" {
		return createParamError("index", "为必填项")
	}
	timestamp := getArg(args, "timestamp")
	if timestamp == "" {
		return createParamError("timestamp", "为必填项")
	}

	timeField := getArg(args, "time_field")
	if timeField == "" {
		timeField = "@timestamp"
	}
	docID := getArg(args, "doc_id")
	beforeCount := getIntArg(args, "before_count", defaultContextCount)
	afterCount := getIntArg(args, "after_count", defaultContextCount)
	maxFieldLen := resolveMaxFieldLen(args)

	var source interface{}
	if src, ok := args["_source"]; ok {
		source = src
	}

	// 额外过滤
	var extraFilters []interface{}
	if f, ok := args["filter"].(map[string]interface{}); ok {
		extraFilters = append(extraFilters, f)
	}

	// 查询之前的日志（时间 <= timestamp，desc 排序取 beforeCount+1）
	beforeFilterList := append([]interface{}{
		map[string]interface{}{
			"range": map[string]interface{}{
				timeField: map[string]interface{}{
					"lte": timestamp, "format": "strict_date_optional_time||epoch_millis",
				},
			},
		},
	}, extraFilters...)

	beforeReq := &elasticsearch.SearchRequestExtended{
		SearchRequest: elasticsearch.SearchRequest{
			Index:  index,
			Query:  map[string]interface{}{"bool": map[string]interface{}{"filter": beforeFilterList}},
			Size:   beforeCount + 1,
			Sort:   buildSortClause(timeField, "desc"),
			Source: source,
		},
	}
	beforeResult, err := client.SearchWithAggs(ctx, beforeReq)
	if err != nil {
		return createESError("查询之前日志", err.Error())
	}

	// 查询之后的日志（时间 >= timestamp，asc 排序取 afterCount+1）
	afterFilterList := append([]interface{}{
		map[string]interface{}{
			"range": map[string]interface{}{
				timeField: map[string]interface{}{
					"gte": timestamp, "format": "strict_date_optional_time||epoch_millis",
				},
			},
		},
	}, extraFilters...)

	afterReq := &elasticsearch.SearchRequestExtended{
		SearchRequest: elasticsearch.SearchRequest{
			Index:  index,
			Query:  map[string]interface{}{"bool": map[string]interface{}{"filter": afterFilterList}},
			Size:   afterCount + 1,
			Sort:   buildSortClause(timeField, "asc"),
			Source: source,
		},
	}
	afterResult, err := client.SearchWithAggs(ctx, afterReq)
	if err != nil {
		return createESError("查询之后日志", err.Error())
	}

	beforeHits := extractHits(*beforeResult)
	afterHits := extractHits(*afterResult)

	// before 是 desc 排序，反转为时间正序
	reverseHits(beforeHits)

	// 截断
	beforeHits = truncateHits(beforeHits, maxFieldLen, nil)
	afterHits = truncateHits(afterHits, maxFieldLen, nil)

	output := map[string]interface{}{
		"index":        index,
		"anchor":       map[string]interface{}{"timestamp": timestamp, "doc_id": docID},
		"before_logs":  beforeHits,
		"after_logs":   afterHits,
		"before_count": len(beforeHits),
		"after_count":  len(afterHits),
	}

	summary := fmt.Sprintf("日志上下文：'%s' 中获取了 %d 条之前 + %d 条之后的日志", index, len(beforeHits), len(afterHits))
	return createSuccessResult(summary, output)
}

// ===== handleFieldStats =====

func (et *ElasticsearchTools) handleFieldStats(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource := getArg(args, "datasource")
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, "连接不可用", et.getAvailableDatasources())
	}

	index := getArg(args, "index")
	if index == "" {
		return createParamError("index", "为必填项")
	}

	timeField := getArg(args, "time_field")
	if timeField == "" {
		timeField = "@timestamp"
	}
	startTime := getArg(args, "start_time")
	endTime := getArg(args, "end_time")
	topN := getIntArg(args, "top_n", 10)

	// 获取字段能力
	fieldCaps, err := client.FieldCaps(ctx, index)
	if err != nil {
		return createESError("获取字段信息", err.Error())
	}
	fields := parseFieldCaps(fieldCaps)

	// 指定字段过滤
	var targetFields []string
	if fArr, ok := args["fields"].([]interface{}); ok {
		for _, f := range fArr {
			if s, ok := f.(string); ok {
				targetFields = append(targetFields, s)
			}
		}
	}

	// 构建聚合
	aggsBody := map[string]interface{}{}
	aggsCount := 0
	maxAggsFields := 20

	for _, field := range fields {
		if aggsCount >= maxAggsFields {
			break
		}
		if len(targetFields) > 0 && !containsString(targetFields, field.Name) {
			continue
		}
		if field.Name == timeField || strings.HasPrefix(field.Name, "_") {
			continue
		}
		if field.Aggregatable && isTermsType(field.Type) {
			aggsBody["field_"+field.Name] = map[string]interface{}{
				"terms": map[string]interface{}{"field": field.Name, "size": topN},
			}
			aggsCount++
		} else if field.Aggregatable && isNumericType(field.Type) {
			aggsBody["field_"+field.Name] = map[string]interface{}{
				"stats": map[string]interface{}{"field": field.Name},
			}
			aggsCount++
		}
	}

	// 执行聚合
	var aggsResult map[string]interface{}
	if len(aggsBody) > 0 {
		searchReq := &elasticsearch.SearchRequestExtended{
			SearchRequest: elasticsearch.SearchRequest{Index: index, Size: 0},
			Aggs:          aggsBody,
		}
		if startTime != "" || endTime != "" {
			searchReq.Query = map[string]interface{}{
				"bool": map[string]interface{}{
					"filter": []interface{}{buildTimeRangeFilter(timeField, startTime, endTime)},
				},
			}
		}
		result, err := client.SearchWithAggs(ctx, searchReq)
		if err != nil {
			return createESError("字段统计查询", err.Error())
		}
		if result != nil {
			aggsResult = *result
		}
	}

	// 组装
	aggregations, _ := aggsResult["aggregations"].(map[string]interface{})
	fieldStats := make([]map[string]interface{}, 0, len(fields))
	for _, field := range fields {
		if len(targetFields) > 0 && !containsString(targetFields, field.Name) {
			continue
		}
		stat := map[string]interface{}{
			"name": field.Name, "type": field.Type,
			"searchable": field.Searchable, "aggregatable": field.Aggregatable,
		}
		if aggregations != nil {
			if agg, ok := aggregations["field_"+field.Name]; ok {
				stat["values"] = agg
			}
		}
		fieldStats = append(fieldStats, stat)
	}

	totalDocs := int64(0)
	if aggsResult != nil {
		totalDocs = extractTotalHits(aggsResult)
	}

	output := map[string]interface{}{
		"index": index, "total_fields": len(fields), "total_docs": totalDocs, "fields": fieldStats,
	}
	return createSuccessResult(
		fmt.Sprintf("索引 '%s' 共 %d 个字段，已统计 %d 个字段的值分布", index, len(fields), aggsCount),
		output,
	)
}

// ===== handleScrollLogs =====

func (et *ElasticsearchTools) handleScrollLogs(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource := getArg(args, "datasource")
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, "连接不可用", et.getAvailableDatasources())
	}

	index := getArg(args, "index")
	if index == "" {
		return createParamError("index", "为必填项")
	}

	keyword := getArg(args, "keyword")
	timeField := getArg(args, "time_field")
	if timeField == "" {
		timeField = "@timestamp"
	}
	startTime := getArg(args, "start_time")
	endTime := getArg(args, "end_time")
	sortOrder := getArg(args, "sort_order")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	size := getIntArg(args, "size", defaultLogSize)
	if size > maxLogSize {
		size = maxLogSize
	}
	maxFieldLen := resolveMaxFieldLen(args)
	compact := isCompactMode(args)

	// 构建查询
	query := buildQueryFromKeyword(keyword)
	var filters []map[string]interface{}
	if startTime != "" || endTime != "" {
		filters = append(filters, buildTimeRangeFilter(timeField, startTime, endTime))
	}
	if f, ok := args["filter"].(map[string]interface{}); ok {
		filters = append(filters, f)
	}
	finalQuery := buildBoolQuery(query, filters...)

	// 构建搜索体（需要支持 search_after，用 MSearch 单请求）
	searchBody := map[string]interface{}{
		"query": finalQuery,
		"size":  size,
		"sort":  buildSortClause(timeField, sortOrder),
	}
	if src, ok := args["_source"]; ok {
		searchBody["_source"] = src
	}
	if sa, ok := args["search_after"].([]interface{}); ok && len(sa) > 0 {
		searchBody["search_after"] = sa
	}

	// 用 MSearch 发送（因为标准 Search 不支持 search_after）
	msearchItems := []elasticsearch.MSearchItem{{Index: index, Body: searchBody}}
	responses, err := client.MSearch(ctx, msearchItems)
	if err != nil {
		return createESError("日志翻页查询", err.Error())
	}
	if len(responses) < 1 {
		return createESError("翻页查询", "返回结果不完整")
	}

	response := responses[0]
	if errInfo, ok := response["error"]; ok {
		return createESError("Elasticsearch", fmt.Sprintf("%v", errInfo))
	}

	totalHits := extractTotalHits(response)
	hits := extractHits(response)
	hits = truncateHits(hits, maxFieldLen, nil)

	var nextSearchAfter interface{}
	if len(hits) > 0 {
		if lastHit, ok := hits[len(hits)-1].(map[string]interface{}); ok {
			nextSearchAfter = lastHit["sort"]
		}
	}
	hasMore := len(hits) == size

	output := map[string]interface{}{
		"total_hits": totalHits, "returned_count": len(hits),
		"has_more": hasMore, "next_search_after": nextSearchAfter,
	}

	if compact {
		output["compact_logs"] = compactHits(hits, getArg(args, "time_field"))
		output["mode"] = "compact"
	} else {
		output["hits"] = hits
		output["mode"] = "full"
	}

	pageInfo := "首页"
	if _, ok := args["search_after"]; ok {
		pageInfo = "翻页"
	}
	moreText := map[bool]string{true: "还有更多", false: "已到末尾"}[hasMore]
	return createSuccessResult(
		fmt.Sprintf("日志%s：索引 '%s'，总命中 %d 条，本页 %d 条，%s", pageInfo, index, totalHits, len(hits), moreText),
		output,
	)
}

// ===== 类型判断辅助 =====

type fieldInfo struct {
	Name         string
	Type         string
	Searchable   bool
	Aggregatable bool
}

func parseFieldCaps(caps map[string]interface{}) []fieldInfo {
	var fields []fieldInfo
	fieldsMap, ok := caps["fields"].(map[string]interface{})
	if !ok {
		return fields
	}
	for name, typesRaw := range fieldsMap {
		typesMap, ok := typesRaw.(map[string]interface{})
		if !ok {
			continue
		}
		for typeName, capsRaw := range typesMap {
			capsMap, ok := capsRaw.(map[string]interface{})
			if !ok {
				continue
			}
			searchable, _ := capsMap["searchable"].(bool)
			aggregatable, _ := capsMap["aggregatable"].(bool)
			fields = append(fields, fieldInfo{
				Name: name, Type: typeName, Searchable: searchable, Aggregatable: aggregatable,
			})
			break
		}
	}
	return fields
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func isTermsType(t string) bool {
	switch t {
	case "keyword", "boolean", "ip":
		return true
	}
	return false
}

func isNumericType(t string) bool {
	switch t {
	case "long", "integer", "float", "double", "short", "byte", "half_float", "scaled_float":
		return true
	}
	return false
}
