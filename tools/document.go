package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ===== 文档操作处理程序 =====

func (et *ElasticsearchTools) handleDocumentGet(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, ok := args["index"].(string)
	if !ok || index == "" {
		return createParamError("index", "为必填项")
	}
	id, ok := args["id"].(string)
	if !ok || id == "" {
		return createParamError("id", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	result, err := client.Get(ctx, index, id)
	if err != nil {
		return createESError("获取文档", err.Error())
	}
	if !result.Found {
		return createNotFoundError("文档", fmt.Sprintf("索引 '%s' 中ID为 '%s'", index, id))
	}
	return createSuccessResult(
		fmt.Sprintf("在数据源 '%s' 中成功获取索引 '%s' 中ID为 '%s' 的文档", datasource, index, id),
		result,
	)
}

func (et *ElasticsearchTools) handleDocumentCount(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, _ := args["index"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	var query map[string]interface{}
	if q, exists := args["query"]; exists {
		if queryMap, ok := q.(map[string]interface{}); ok {
			query = queryMap
		}
	}
	result, err := client.Count(ctx, index, query)
	if err != nil {
		return createESError("统计文档数量", err.Error())
	}
	desc := fmt.Sprintf("数据源 '%s' 文档数量: %d", datasource, result.Count)
	if index != "" {
		desc = fmt.Sprintf("数据源 '%s' 索引 '%s' 文档数量: %d", datasource, index, result.Count)
	}
	return createSuccessResult(desc, result)
}

func (et *ElasticsearchTools) handleDocumentIndex(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, ok := args["index"].(string)
	if !ok || index == "" {
		return createParamError("index", "为必填项")
	}
	body, ok := args["body"].(map[string]interface{})
	if !ok {
		return createParamError("body", "为必填项，且必须为 JSON 对象")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	id, _ := args["id"].(string)
	result, err := client.Index(ctx, index, id, body)
	if err != nil {
		return createESError("索引文档", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("在数据源 '%s' 中文档索引成功", datasource), result)
}
