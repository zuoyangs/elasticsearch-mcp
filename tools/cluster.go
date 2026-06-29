// Package tools 提供集群与实例相关的处理器函数。
package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// handleListInstances 列出所有已配置的数据源实例
func (et *ElasticsearchTools) handleListInstances(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	names := et.getAvailableDatasources()
	result := map[string]interface{}{
		"instances":          names,
		"count":              len(names),
		"default_datasource": et.defaultDatasource,
	}
	return createSuccessResult(fmt.Sprintf("共 %d 个已配置的数据源实例", len(names)), result)
}

// handleClusterInfo 获取集群信息
func (et *ElasticsearchTools) handleClusterInfo(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	info, err := client.Info(ctx)
	if err != nil {
		return createESError("获取集群信息", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 集群信息检索成功", datasource), info)
}

// handleClusterHealth 获取集群健康状态
func (et *ElasticsearchTools) handleClusterHealth(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	health, err := client.Health(ctx)
	if err != nil {
		return createESError("获取集群健康状态", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 集群健康状态检索成功", datasource), health)
}

// handleClusterStats 获取集群统计信息
func (et *ElasticsearchTools) handleClusterStats(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	stats, err := client.ClusterStats(ctx)
	if err != nil {
		return createESError("获取集群统计信息", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 集群统计信息检索成功", datasource), stats)
}

// handleNodesInfo 获取节点信息
func (et *ElasticsearchTools) handleNodesInfo(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	nodeID, _ := args["node_id"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	info, err := client.NodesInfo(ctx, nodeID)
	if err != nil {
		return createESError("获取节点信息", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 节点信息检索成功", datasource), info)
}

// handleNodesStats 获取节点统计信息
func (et *ElasticsearchTools) handleNodesStats(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	nodeID, _ := args["node_id"].(string)
	metric, _ := args["metric"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	stats, err := client.NodesStats(ctx, nodeID, metric)
	if err != nil {
		return createESError("获取节点统计信息", err.Error())
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 节点统计信息检索成功", datasource), stats)
}

// handleResolveNode 解析节点状态，判断节点在线或离线
func (et *ElasticsearchTools) handleResolveNode(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	targetNodeID, _ := args["node_id"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	// 先尝试直接查询该节点（如果还在线）
	info, err := client.NodesInfo(ctx, targetNodeID)
	if err == nil {
		if nodes, ok := info["nodes"].(map[string]interface{}); ok {
			if nodeData, exists := nodes[targetNodeID]; exists {
				if nd, ok := nodeData.(map[string]interface{}); ok {
					result := map[string]interface{}{
						"node_id":   targetNodeID,
						"node_name": nd["name"],
						"status":    "online",
						"roles":     nd["roles"],
						"host":      nd["host"],
						"ip":        nd["ip"],
					}
					if transport, ok := nd["transport_address"].(string); ok {
						result["transport_address"] = transport
					}
					return createSuccessResult(fmt.Sprintf("节点 %s 当前在线", targetNodeID), result)
				}
			}
		}
	}
	// 节点不在线，返回所有在线节点列表供对比
	allInfo, err := client.NodesInfo(ctx, "")
	if err != nil {
		return createESError("获取节点信息", err.Error())
	}
	nodeList := []map[string]interface{}{}
	if nodes, ok := allInfo["nodes"].(map[string]interface{}); ok {
		for id, nodeData := range nodes {
			if nd, ok := nodeData.(map[string]interface{}); ok {
				nodeList = append(nodeList, map[string]interface{}{
					"id":   id,
					"name": nd["name"],
					"ip":   nd["ip"],
				})
			}
		}
	}
	result := map[string]interface{}{
		"node_id":      targetNodeID,
		"status":       "offline（节点已离开集群，无法查询其名称）",
		"online_nodes": nodeList,
		"hint":         "该节点已离线。可通过 Rancher 查看 namespace 下最近重启的 pod 来确认是哪个节点。",
	}
	return createSuccessResult(fmt.Sprintf("节点 %s 已离线，返回当前在线节点列表供对比", targetNodeID), result)
}
