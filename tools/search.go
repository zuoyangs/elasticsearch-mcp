package tools

import (
	"context"
	"fmt"

	"elasticsearch-mcp/elasticsearch"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ===== 搜索操作处理程序 =====

func (et *ElasticsearchTools) handleSearch(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	index, _ := args["index"].(string)

	query := map[string]interface{}{"match_all": map[string]interface{}{}}
	if q, exists := args["query"]; exists {
		if queryMap, ok := q.(map[string]interface{}); ok {
			query = queryMap
		}
	}

	size := 10
	if s, exists := args["size"]; exists {
		if sizeFloat, ok := s.(float64); ok {
			size = int(sizeFloat)
		}
	}

	from := 0
	if f, exists := args["from"]; exists {
		if fromFloat, ok := f.(float64); ok {
			from = int(fromFloat)
		}
	}

	var sort []interface{}
	if s, exists := args["sort"]; exists {
		if sortArray, ok := s.([]interface{}); ok {
			sort = sortArray
		}
	}

	var source interface{}
	if src, exists := args["_source"]; exists {
		source = src
	}

	var trackTotalHits *bool
	if tth, exists := args["track_total_hits"]; exists {
		if b, ok := tth.(bool); ok {
			trackTotalHits = &b
		}
	}

	aggs, hasAggs := args["aggs"].(map[string]interface{})
	highlight, hasHighlight := args["highlight"].(map[string]interface{})

	if hasAggs || hasHighlight {
		extReq := &elasticsearch.SearchRequestExtended{
			SearchRequest: elasticsearch.SearchRequest{
				Index:          index,
				Query:          query,
				Size:           size,
				From:           from,
				Sort:           sort,
				Source:         source,
				TrackTotalHits: trackTotalHits,
			},
			Aggs:      aggs,
			Highlight: highlight,
		}
		result, err := client.SearchWithAggs(ctx, extReq)
		if err != nil {
			return createESError("搜索", err.Error())
		}
		return createSuccessResult(fmt.Sprintf("在数据源 '%s' 中搜索执行成功", datasource), result)
	}

	searchRequest := &elasticsearch.SearchRequest{
		Index:          index,
		Query:          query,
		Size:           size,
		From:           from,
		Sort:           sort,
		Source:         source,
		TrackTotalHits: trackTotalHits,
	}
	result, err := client.Search(ctx, searchRequest)
	if err != nil {
		return createESError("搜索", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("在数据源 '%s' 中搜索执行成功", datasource), result)
}

func (et *ElasticsearchTools) handleSQLQuery(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return createParamError("query", "为必填项")
	}
	format, _ := args["format"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	result, err := client.SQLQuery(ctx, query, format)
	if err != nil {
		return createESError("SQL 查询", err.Error())
	}
	if text, ok := result.(string); ok {
		return createSimpleSuccessResult(text)
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' SQL 查询执行成功", datasource), result)
}

func (et *ElasticsearchTools) handleSQLTranslate(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return createParamError("query", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	result, err := client.SQLTranslate(ctx, query)
	if err != nil {
		return createESError("SQL 翻译", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' SQL 翻译成功", datasource), result)
}
