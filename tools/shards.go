package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ===== 分片与段处理程序 =====

func (et *ElasticsearchTools) handleCatShards(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, _ := args["index"].(string)
	if index == "" {
		return createParamError("index", "为必填项。大规模集群下不指定索引会返回全量分片数据（可能 20w+），请指定 index 参数，支持通配符如 logs-2024.01.*")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	shards, err := client.CatShards(ctx, index)
	if err != nil {
		return createESError("获取分片信息", err.Error())
	}

	if len(shards) <= 200 {
		return createSuccessResult(fmt.Sprintf("数据源 '%s' 索引 '%s' 分片信息，共 %d 个", datasource, index, len(shards)), shards)
	}

	result := summarizeShards(shards)
	result["datasource"] = datasource
	result["index_pattern"] = index
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 索引 '%s' 共 %d 个分片（已生成摘要）", datasource, index, len(shards)), result)
}

func (et *ElasticsearchTools) handleCatSegments(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, _ := args["index"].(string)
	if index == "" {
		return createParamError("index", "为必填项。大规模集群下不指定索引会返回全量段数据，请指定 index 参数，支持通配符如 logs-2024.01.*")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	segments, err := client.CatSegments(ctx, index)
	if err != nil {
		return createESError("获取段信息", err.Error())
	}

	if len(segments) <= 200 {
		return createSuccessResult(fmt.Sprintf("数据源 '%s' 索引 '%s' 段信息，共 %d 个", datasource, index, len(segments)), segments)
	}

	result := summarizeSegments(segments)
	result["datasource"] = datasource
	result["index_pattern"] = index
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 索引 '%s' 共 %d 个段（已生成摘要）", datasource, index, len(segments)), result)
}

// ===== 任务管理处理程序 =====

func (et *ElasticsearchTools) handleListTasks(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	detailed := false
	if d, exists := args["detailed"]; exists {
		if dBool, ok := d.(bool); ok {
			detailed = dBool
		}
	}
	actions, _ := args["actions"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	tasks, err := client.ListTasks(ctx, detailed, actions)
	if err != nil {
		return createESError("列出任务", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 任务列表检索成功", datasource), tasks)
}

func (et *ElasticsearchTools) handleGetTask(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return createParamError("task_id", "为必填项，格式为 node_id:task_number")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	task, err := client.GetTask(ctx, taskID)
	if err != nil {
		return createESError("获取任务详情", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 任务 '%s' 详情检索成功", datasource, taskID), task)
}

// ===== 模板管理处理程序 =====

func (et *ElasticsearchTools) handleListTemplates(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	templates, err := client.ListTemplates(ctx)
	if err != nil {
		return createESError("列出模板", err.Error())
	}
	result := map[string]interface{}{
		"datasource": datasource,
		"templates":  templates,
		"count":      len(templates),
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 共 %d 个索引模板", datasource, len(templates)), result)
}

func (et *ElasticsearchTools) handleGetTemplate(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return createParamError("name", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	template, err := client.GetTemplate(ctx, name)
	if err != nil {
		return createESError("获取模板详情", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 模板 '%s' 详情检索成功", datasource, name), template)
}

// ===== 快照管理处理程序 =====

func (et *ElasticsearchTools) handleListRepositories(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	repos, err := client.ListRepositories(ctx)
	if err != nil {
		return createESError("列出快照仓库", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 快照仓库列表检索成功", datasource), repos)
}

func (et *ElasticsearchTools) handleListSnapshots(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	repository, ok := args["repository"].(string)
	if !ok || repository == "" {
		return createParamError("repository", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	snapshots, err := client.ListSnapshots(ctx, repository)
	if err != nil {
		return createESError("列出快照", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 仓库 '%s' 快照列表检索成功", datasource, repository), snapshots)
}

// ===== ILM 处理程序 =====

func (et *ElasticsearchTools) handleListILMPolicies(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	policies, err := client.ListILMPolicies(ctx)
	if err != nil {
		return createESError("列出 ILM 策略", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' ILM 策略列表检索成功", datasource), policies)
}

func (et *ElasticsearchTools) handleGetILMStatus(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, ok := args["index"].(string)
	if !ok || index == "" {
		return createParamError("index", "为必填项")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	status, err := client.GetILMStatus(ctx, index)
	if err != nil {
		return createESError("获取 ILM 状态", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 索引 '%s' ILM 状态检索成功", datasource, index), status)
}

// ===== SRE 场景化诊断处理程序 =====

func (et *ElasticsearchTools) handleCatAllocation(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	allocation, err := client.CatAllocation(ctx)
	if err != nil {
		return createESError("获取分片分配信息", err.Error())
	}

	result := summarizeAllocation(allocation)
	result["datasource"] = datasource
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 分片分配信息，共 %d 个节点", datasource, len(allocation)), result)
}

func (et *ElasticsearchTools) handleCatThreadPool(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	threadPool, _ := args["thread_pool"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	pools, err := client.CatThreadPool(ctx, threadPool)
	if err != nil {
		return createESError("获取线程池信息", err.Error())
	}

	result := summarizeThreadPools(pools)
	result["datasource"] = datasource
	if threadPool != "" {
		result["filter"] = threadPool
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 线程池信息，共 %d 条记录", datasource, len(pools)), result)
}

func (et *ElasticsearchTools) handleCatPendingTasks(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	tasks, err := client.CatPendingTasks(ctx)
	if err != nil {
		return createESError("获取待处理任务", err.Error())
	}

	result := summarizePendingTasks(tasks)
	result["datasource"] = datasource
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 待处理任务，共 %d 个", datasource, len(tasks)), result)
}

func (et *ElasticsearchTools) handleCatRecovery(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	index, _ := args["index"].(string)
	if index == "" {
		return createParamError("index", "为必填项。大规模集群下不指定索引会返回全量恢复数据，请指定 index 参数，支持通配符如 logs-2024.01.*")
	}
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	recovery, err := client.CatRecovery(ctx, index)
	if err != nil {
		return createESError("获取恢复信息", err.Error())
	}

	if len(recovery) <= 100 {
		return createSuccessResult(fmt.Sprintf("数据源 '%s' 索引 '%s' 恢复信息，共 %d 条", datasource, index, len(recovery)), recovery)
	}

	result := summarizeRecovery(recovery)
	result["datasource"] = datasource
	result["index_pattern"] = index
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 索引 '%s' 恢复信息，共 %d 条（已生成摘要）", datasource, index, len(recovery)), result)
}

func (et *ElasticsearchTools) handleNodesHotThreads(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	nodeID, _ := args["node_id"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	hotThreads, err := client.NodesHotThreads(ctx, nodeID)
	if err != nil {
		return createESError("获取热线程信息", err.Error())
	}
	return createSimpleSuccessResult(fmt.Sprintf("数据源 '%s' 热线程信息:\n%s", datasource, hotThreads))
}
