// Package tools - index.go 提供索引和别名操作的处理器函数。
// 这些处理器使用标准化的错误码辅助函数，确保错误信息结构化且易于诊断。
package tools

import (
	"context"
	"fmt"

	"elasticsearch-mcp/elasticsearch"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// handleIndexList 列出集群中的索引
func (et *ElasticsearchTools) handleIndexList(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	pattern, _ := args["pattern"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	var indices []elasticsearch.IndexInfo
	if pattern != "" {
		indices, err = client.ListIndicesWithPattern(ctx, pattern)
	} else {
		indices, err = client.ListIndices(ctx)
	}
	if err != nil {
		return createESError("列出索引", err.Error())
	}
	if len(indices) <= 200 {
		result := map[string]interface{}{
			"datasource": datasource,
			"count":      len(indices),
			"indices":    indices,
		}
		return createSuccessResult(fmt.Sprintf("在数据源 '%s' 中找到 %d 个索引", datasource, len(indices)), result)
	}
	result := summarizeIndices(indices)
	result["datasource"] = datasource
	return createSuccessResult(fmt.Sprintf("在数据源 '%s' 中找到 %d 个索引（已生成摘要）", datasource, len(indices)), result)
}

// handleIndexExists 检查索引是否存在
func (et *ElasticsearchTools) handleIndexExists(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, ok := args["index"].(string)
	if !ok {
		return createParamError("index", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	exists, err := client.IndexExists(ctx, index)
	if err != nil {
		return createESError("检查索引存在性", err.Error())
	}
	result := map[string]interface{}{
		"datasource": datasource,
		"index":      index,
		"exists":     exists,
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 中索引 '%s' 存在: %t", datasource, index, exists), result)
}

// handleGetIndex 获取索引详情
func (et *ElasticsearchTools) handleGetIndex(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, ok := args["index"].(string)
	if !ok {
		return createParamError("index", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	detail, err := client.GetIndex(ctx, index)
	if err != nil {
		return createESError("获取索引详情", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 中索引 '%s' 详情检索成功", datasource, index), detail)
}

// handleGetMapping 获取索引映射
func (et *ElasticsearchTools) handleGetMapping(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, ok := args["index"].(string)
	if !ok {
		return createParamError("index", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	mapping, err := client.GetMapping(ctx, index)
	if err != nil {
		return createESError("获取索引映射", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 中索引 '%s' 映射检索成功", datasource, index), mapping)
}

// handleGetSettings 获取索引设置
func (et *ElasticsearchTools) handleGetSettings(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, ok := args["index"].(string)
	if !ok {
		return createParamError("index", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	settings, err := client.GetSettings(ctx, index)
	if err != nil {
		return createESError("获取索引设置", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 中索引 '%s' 设置检索成功", datasource, index), settings)
}

// handleIndexStats 获取索引统计信息
func (et *ElasticsearchTools) handleIndexStats(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, _ := args["index"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	stats, err := client.IndexStats(ctx, index)
	if err != nil {
		return createESError("获取索引统计信息", err.Error())
	}
	desc := fmt.Sprintf("数据源 '%s' 索引统计信息检索成功", datasource)
	if index != "" {
		desc = fmt.Sprintf("数据源 '%s' 中索引 '%s' 统计信息检索成功", datasource, index)
	}
	return createSuccessResult(desc, stats)
}

// handleIndexCreate 创建索引
func (et *ElasticsearchTools) handleIndexCreate(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, ok := args["index"].(string)
	if !ok {
		return createParamError("index", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	body := make(map[string]interface{})
	if settings, exists := args["settings"]; exists {
		if settingsMap, ok := settings.(map[string]interface{}); ok {
			body["settings"] = settingsMap
		}
	}
	if mappings, exists := args["mappings"]; exists {
		if mappingsMap, ok := mappings.(map[string]interface{}); ok {
			body["mappings"] = mappingsMap
		}
	}
	err = client.CreateIndex(ctx, index, body)
	if err != nil {
		return createESError("创建索引", err.Error())
	}
	return createSimpleSuccessResult(fmt.Sprintf("在数据源 '%s' 中索引 '%s' 创建成功", datasource, index))
}

// handleOpenIndex 打开索引
func (et *ElasticsearchTools) handleOpenIndex(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, ok := args["index"].(string)
	if !ok {
		return createParamError("index", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	err = client.OpenIndex(ctx, index)
	if err != nil {
		return createESError("打开索引", err.Error())
	}
	return createSimpleSuccessResult(fmt.Sprintf("在数据源 '%s' 中索引 '%s' 已成功打开", datasource, index))
}

// handleCloseIndex 关闭索引
func (et *ElasticsearchTools) handleCloseIndex(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, ok := args["index"].(string)
	if !ok {
		return createParamError("index", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	err = client.CloseIndex(ctx, index)
	if err != nil {
		return createESError("关闭索引", err.Error())
	}
	return createSimpleSuccessResult(fmt.Sprintf("在数据源 '%s' 中索引 '%s' 已成功关闭", datasource, index))
}

// handleListAliases 列出别名
func (et *ElasticsearchTools) handleListAliases(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, _ := args["index"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	aliases, err := client.ListAliases(ctx, index)
	if err != nil {
		return createESError("列出别名", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 别名列表检索成功", datasource), aliases)
}

// handleGetAlias 获取别名详情
func (et *ElasticsearchTools) handleGetAlias(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	alias, ok := args["alias"].(string)
	if !ok {
		return createParamError("alias", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	result, err := client.GetAlias(ctx, alias)
	if err != nil {
		return createESError("获取别名详情", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 别名 '%s' 详情检索成功", datasource, alias), result)
}
