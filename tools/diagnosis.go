package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"elasticsearch-mcp/elasticsearch"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ===== 集群健康分析处理程序 =====

func (et *ElasticsearchTools) handleClusterHealthAnalysis(ctx context.Context, args map[string]interface{}) mcp.CallToolResult {
	datasource, _ := args["datasource"].(string)
	client, err := et.getClientWithConnectionCheck(ctx, datasource)
	if err != nil {
		return createDatasourceError(datasource, err.Error(), et.getAvailableDatasources())
	}
	health, err := client.GetDetailedHealth(ctx)
	if err != nil {
		return createESError("获取集群详细健康状态", err.Error())
	}

	// 构建 nodeID → nodeName 映射表
	nodeIDToName := map[string]string{}
	nodesInfo, err := client.NodesInfo(ctx, "")
	if err == nil {
		if nodes, ok := nodesInfo["nodes"].(map[string]interface{}); ok {
			for nodeID, nodeData := range nodes {
				if nd, ok := nodeData.(map[string]interface{}); ok {
					if name, ok := nd["name"].(string); ok {
						nodeIDToName[nodeID] = name
					}
				}
			}
		}
	}

	analysis := analyzeClusterHealth(health)

	if health.Status != "green" && health.UnassignedShards > 0 {
		allocExplain, err := client.ClusterAllocationExplain(ctx, "", 0, false)
		if err != nil {
			analysis["allocation_explain_error"] = fmt.Sprintf("获取分配解释失败: %v", err)
		} else {
			parsed := parseAllocationExplain(allocExplain)
			if details, ok := parsed["unassigned_details"].(string); ok && parsed["unassigned_reason"] == "NODE_LEFT" {
				for nodeID, nodeName := range nodeIDToName {
					if strings.Contains(details, nodeID) {
						parsed["left_node_id"] = nodeID
						parsed["left_node_name"] = nodeName
						break
					}
				}
				if _, found := parsed["left_node_name"]; !found {
					if currentNode, ok := allocExplain["current_node"].(map[string]interface{}); ok {
						if name, ok := currentNode["name"].(string); ok {
							parsed["left_node_name"] = name
						}
						if id, ok := currentNode["id"].(string); ok {
							parsed["left_node_id"] = id
						}
					}
				}
			}
			analysis["allocation_explain"] = parsed
		}

		unassignedByIndex := aggregateUnassignedFromHealth(health)
		if len(unassignedByIndex) > 30 {
			analysis["unassigned_by_index"] = unassignedByIndex[:30]
			analysis["unassigned_by_index_total"] = len(unassignedByIndex)
			analysis["unassigned_by_index_note"] = fmt.Sprintf("共 %d 个索引有未分配分片，仅展示前 30 个（按未分配数降序）", len(unassignedByIndex))
		} else {
			analysis["unassigned_by_index"] = unassignedByIndex
		}

		allocation, err := client.CatAllocation(ctx)
		if err != nil {
			analysis["disk_allocation_error"] = fmt.Sprintf("获取磁盘分配信息失败: %v", err)
		} else {
			diskAnalysis := analyzeDiskWatermark(allocation)
			if len(diskAnalysis) > 20 {
				analysis["disk_watermark_issues"] = diskAnalysis[:20]
				analysis["disk_watermark_issues_total"] = len(diskAnalysis)
				analysis["disk_watermark_note"] = fmt.Sprintf("共 %d 个节点存在磁盘水位线问题，仅展示前 20 个", len(diskAnalysis))
			} else if len(diskAnalysis) > 0 {
				analysis["disk_watermark_issues"] = diskAnalysis
			}
			analysis["disk_watermark_summary"] = summarizeDiskWatermark(allocation)
		}

		if health.Status == "red" {
			redExplains := []map[string]interface{}{}
			count := 0
			for indexName, indexHealth := range health.Indices {
				if indexHealth.Status == "red" && count < 3 {
					explain, err := client.ClusterAllocationExplain(ctx, indexName, 0, true)
					if err != nil {
						continue
					}
					parsed := parseAllocationExplain(explain)
					parsed["index"] = indexName
					parsed["shard"] = 0
					parsed["type"] = "primary"
					if details, ok := parsed["unassigned_details"].(string); ok {
						for nodeID, nodeName := range nodeIDToName {
							if strings.Contains(details, nodeID) {
								parsed["left_node_id"] = nodeID
								parsed["left_node_name"] = nodeName
								break
							}
						}
					}
					redExplains = append(redExplains, parsed)
					count++
				}
			}
			if len(redExplains) > 0 {
				analysis["red_index_shard_explains"] = redExplains
			}
		}
	}

	healthSummary := map[string]interface{}{
		"cluster_name":                    health.ClusterName,
		"status":                          health.Status,
		"timed_out":                       health.TimedOut,
		"number_of_nodes":                 health.NumberOfNodes,
		"number_of_data_nodes":            health.NumberOfDataNodes,
		"active_primary_shards":           health.ActivePrimaryShards,
		"active_shards":                   health.ActiveShards,
		"relocating_shards":               health.RelocatingShards,
		"initializing_shards":             health.InitializingShards,
		"unassigned_shards":               health.UnassignedShards,
		"delayed_unassigned_shards":       health.DelayedUnassignedShards,
		"number_of_pending_tasks":         health.NumberOfPendingTasks,
		"active_shards_percent_as_number": health.ActiveShardsPercentAsNumber,
		"total_indices_count":             len(health.Indices),
	}
	nonGreenIndices := map[string]elasticsearch.IndexHealth{}
	redCount, yellowCount := 0, 0
	for name, idx := range health.Indices {
		if idx.Status == "red" {
			redCount++
		} else if idx.Status == "yellow" {
			yellowCount++
		}
		if idx.Status == "red" && len(nonGreenIndices) < 30 {
			nonGreenIndices[name] = idx
		}
	}
	if len(nonGreenIndices) < 30 {
		for name, idx := range health.Indices {
			if idx.Status == "yellow" && len(nonGreenIndices) < 30 {
				nonGreenIndices[name] = idx
			}
		}
	}
	if redCount+yellowCount > 0 {
		healthSummary["non_green_indices_sample"] = nonGreenIndices
		healthSummary["red_indices_count"] = redCount
		healthSummary["yellow_indices_count"] = yellowCount
		healthSummary["non_green_indices_total"] = redCount + yellowCount
		if redCount+yellowCount > 30 {
			healthSummary["non_green_indices_note"] = fmt.Sprintf("共 %d 个非 green 索引（%d red + %d yellow），仅展示前 30 个", redCount+yellowCount, redCount, yellowCount)
		}
	}

	result := map[string]interface{}{
		"datasource":    datasource,
		"clusterHealth": healthSummary,
		"analysis":      analysis,
	}
	if health.Status != "green" && len(nodeIDToName) > 0 {
		nodeMap := map[string]string{}
		count := 0
		for id, name := range nodeIDToName {
			nodeMap[id] = name
			count++
			if count >= 20 {
				break
			}
		}
		result["node_id_to_name_sample"] = nodeMap
		result["total_online_nodes"] = len(nodeIDToName)
	}
	return createSuccessResult(fmt.Sprintf("数据源 '%s' 集群健康状态深度分析完成 - 当前状态: %s", datasource, health.Status), result)
}

// ===== 集群健康分析辅助函数 =====

func analyzeClusterHealth(health *elasticsearch.ClusterHealthDetailedResponse) map[string]interface{} {
	analysis := map[string]interface{}{
		"current_status": health.Status,
		"summary":        "",
		"risk_level":     "",
		"indicators":     []string{},
		"root_causes":    []string{},
		"recommendations": []string{},
	}

	indicators := []string{}
	rootCauses := []string{}
	recommendations := []string{}

	analysis["active_shards_percent"] = fmt.Sprintf("%.2f%%", health.ActiveShardsPercentAsNumber)
	analysis["cluster_overview"] = map[string]interface{}{
		"nodes":               health.NumberOfNodes,
		"data_nodes":          health.NumberOfDataNodes,
		"active_shards":       health.ActiveShards,
		"active_primary":      health.ActivePrimaryShards,
		"relocating_shards":   health.RelocatingShards,
		"initializing_shards": health.InitializingShards,
		"unassigned_shards":   health.UnassignedShards,
		"delayed_unassigned":  health.DelayedUnassignedShards,
		"pending_tasks":       health.NumberOfPendingTasks,
	}

	if health.UnassignedShards > 0 {
		indicators = append(indicators, "unassigned_shards")
		rootCauses = append(rootCauses,
			fmt.Sprintf("存在 %d 个未分配分片（其中延迟分配 %d 个），这是集群状态非 green 的直接原因",
				health.UnassignedShards, health.DelayedUnassignedShards))
	}
	if health.InitializingShards > 0 {
		indicators = append(indicators, "initializing_shards")
		rootCauses = append(rootCauses,
			fmt.Sprintf("有 %d 个分片正在初始化，集群可能正在恢复中", health.InitializingShards))
	}
	if health.RelocatingShards > 0 {
		indicators = append(indicators, "relocating_shards")
		rootCauses = append(rootCauses,
			fmt.Sprintf("有 %d 个分片正在重定位，可能是节点加入/离开或手动 rebalance", health.RelocatingShards))
	}
	if health.NumberOfPendingTasks > 10 {
		indicators = append(indicators, "high_pending_tasks")
		rootCauses = append(rootCauses,
			fmt.Sprintf("待处理任务数 %d 偏高（最大等待 %dms），master 节点可能压力较大",
				health.NumberOfPendingTasks, health.TaskMaxWaitingInQueueMillis))
	}

	yellowIndices := []map[string]interface{}{}
	redIndices := []map[string]interface{}{}

	for indexName, indexHealth := range health.Indices {
		indexDetail := map[string]interface{}{
			"index":            indexName,
			"status":           indexHealth.Status,
			"shards":           indexHealth.NumberOfShards,
			"replicas":         indexHealth.NumberOfReplicas,
			"active_primary":   indexHealth.ActivePrimaryShards,
			"active_shards":    indexHealth.ActiveShards,
			"unassigned":       indexHealth.UnassignedShards,
			"initializing":     indexHealth.InitializingShards,
			"relocating":       indexHealth.RelocatingShards,
		}

		if indexHealth.Status == "yellow" {
			expectedTotal := indexHealth.NumberOfShards * (1 + indexHealth.NumberOfReplicas)
			missingReplicas := expectedTotal - indexHealth.ActiveShards
			indexDetail["reason"] = fmt.Sprintf("副本分片未完全分配：期望 %d 个分片（%d主×%d副本），实际活跃 %d 个，缺少 %d 个副本分片",
				expectedTotal, indexHealth.NumberOfShards, indexHealth.NumberOfReplicas+1, indexHealth.ActiveShards, missingReplicas)
			if indexHealth.NumberOfReplicas > 0 && health.NumberOfDataNodes <= 1 {
				indexDetail["likely_cause"] = "单数据节点集群无法分配副本分片（副本不能与主分片在同一节点）"
			} else if indexHealth.NumberOfReplicas >= health.NumberOfDataNodes {
				indexDetail["likely_cause"] = fmt.Sprintf("副本数(%d)≥数据节点数(%d)，部分副本无法分配到不同节点",
					indexHealth.NumberOfReplicas, health.NumberOfDataNodes)
			}
			yellowIndices = append(yellowIndices, indexDetail)
		} else if indexHealth.Status == "red" {
			missingPrimary := indexHealth.NumberOfShards - indexHealth.ActivePrimaryShards
			indexDetail["reason"] = fmt.Sprintf("主分片未完全分配：期望 %d 个主分片，实际活跃 %d 个，缺少 %d 个主分片，部分数据不可用",
				indexHealth.NumberOfShards, indexHealth.ActivePrimaryShards, missingPrimary)
			indexDetail["data_loss_risk"] = "高 - 主分片丢失意味着该分片上的数据当前不可读写"
			redIndices = append(redIndices, indexDetail)
		}
	}

	if len(yellowIndices) > 0 {
		indicators = append(indicators, "yellow_indices")
		if len(yellowIndices) > 20 {
			analysis["yellow_indices"] = yellowIndices[:20]
			analysis["yellow_indices_note"] = fmt.Sprintf("共 %d 个 yellow 索引，仅展示前 20 个", len(yellowIndices))
		} else {
			analysis["yellow_indices"] = yellowIndices
		}
		analysis["yellow_indices_count"] = len(yellowIndices)
	}
	if len(redIndices) > 0 {
		indicators = append(indicators, "red_indices")
		if len(redIndices) > 20 {
			analysis["red_indices"] = redIndices[:20]
			analysis["red_indices_note"] = fmt.Sprintf("共 %d 个 red 索引，仅展示前 20 个", len(redIndices))
		} else {
			analysis["red_indices"] = redIndices
		}
		analysis["red_indices_count"] = len(redIndices)
	}

	switch health.Status {
	case "green":
		analysis["summary"] = "集群状态 GREEN：所有主分片和副本分片均已成功分配，集群完全健康"
		analysis["risk_level"] = "无风险"
	case "yellow":
		analysis["summary"] = fmt.Sprintf("集群状态 YELLOW：所有主分片已分配（数据可用），但有 %d 个副本分片未分配（容错能力下降）。共 %d 个索引处于 yellow 状态",
			health.UnassignedShards, len(yellowIndices))
		analysis["risk_level"] = "中等 - 数据可用但无冗余保护，节点故障可能导致数据丢失"
		recommendations = append(recommendations,
			"检查数据节点数是否足够：副本分片不能与对应的主分片在同一节点上",
			"如果是单节点集群或测试环境，可将副本数设为 0：PUT /<index>/_settings {\"number_of_replicas\": 0}",
			"检查磁盘水位线：磁盘使用率 >85% 时 ES 会停止分配分片",
			"检查分片分配过滤规则：index.routing.allocation.* 设置可能阻止分片分配到可用节点",
		)
	case "red":
		analysis["summary"] = fmt.Sprintf("集群状态 RED：有 %d 个主分片未分配，部分数据不可用！共 %d 个索引处于 red 状态，%d 个索引处于 yellow 状态",
			health.UnassignedShards, len(redIndices), len(yellowIndices))
		analysis["risk_level"] = "严重 - 存在数据不可用，需要立即处理"
		recommendations = append(recommendations,
			"【紧急】检查是否有数据节点宕机或离线：对比 number_of_nodes 与预期值",
			"【紧急】查看 _cluster/allocation/explain 了解主分片无法分配的具体原因",
			"检查节点磁盘空间：磁盘满会导致分片无法分配",
			"检查是否有节点 JVM OOM 或频繁 Full GC 导致节点不稳定",
			"如果是旧索引且数据可丢弃，可考虑删除 red 索引恢复集群状态",
			"如果节点数据丢失，可尝试 POST _cluster/reroute?retry_failed=true 重试分配",
		)
	}

	analysis["indicators"] = indicators
	analysis["root_causes"] = rootCauses
	analysis["recommendations"] = recommendations

	return analysis
}

// parseAllocationExplain 解析 _cluster/allocation/explain 的响应
func parseAllocationExplain(explain map[string]interface{}) map[string]interface{} {
	parsed := map[string]interface{}{}

	if index, ok := explain["index"].(string); ok {
		parsed["index"] = index
	}
	if shard, ok := explain["shard"].(float64); ok {
		parsed["shard"] = int(shard)
	}
	if primary, ok := explain["primary"].(bool); ok {
		parsed["primary"] = primary
	}
	if currentState, ok := explain["current_state"].(string); ok {
		parsed["current_state"] = currentState
	}

	if unassignedInfo, ok := explain["unassigned_info"].(map[string]interface{}); ok {
		reason, _ := unassignedInfo["reason"].(string)
		at, _ := unassignedInfo["at"].(string)
		details, _ := unassignedInfo["details"].(string)

		parsed["unassigned_reason"] = reason
		parsed["unassigned_at"] = at
		if details != "" {
			parsed["unassigned_details"] = details
		}

		switch reason {
		case "INDEX_CREATED":
			parsed["reason_explanation"] = "索引刚创建，分片等待分配"
		case "CLUSTER_RECOVERED":
			parsed["reason_explanation"] = "集群恢复后分片等待重新分配"
		case "INDEX_REOPENED":
			parsed["reason_explanation"] = "索引重新打开后分片等待分配"
		case "DANGLING_INDEX_IMPORTED":
			parsed["reason_explanation"] = "悬挂索引被导入，分片等待分配"
		case "NEW_INDEX_RESTORED":
			parsed["reason_explanation"] = "从快照恢复的新索引，分片等待分配"
		case "EXISTING_INDEX_RESTORED":
			parsed["reason_explanation"] = "从快照恢复的已有索引，分片等待分配"
		case "REPLICA_ADDED":
			parsed["reason_explanation"] = "新增副本分片等待分配"
		case "ALLOCATION_FAILED":
			parsed["reason_explanation"] = "分片分配失败（可能是磁盘空间不足、节点过滤规则等）"
		case "NODE_LEFT":
			parsed["reason_explanation"] = "持有该分片的节点离开集群，分片需要重新分配"
		case "REROUTE_CANCELLED":
			parsed["reason_explanation"] = "分片重路由被取消"
		case "REINITIALIZED":
			parsed["reason_explanation"] = "分片重新初始化"
		case "REALLOCATED_REPLICA":
			parsed["reason_explanation"] = "副本分片被重新分配"
		case "PRIMARY_FAILED":
			parsed["reason_explanation"] = "主分片失败，需要从副本提升或重新分配"
		case "FORCED_EMPTY_PRIMARY":
			parsed["reason_explanation"] = "强制分配空主分片（数据丢失）"
		case "MANUAL_ALLOCATION":
			parsed["reason_explanation"] = "手动分配操作"
		default:
			parsed["reason_explanation"] = reason
		}
	}

	if canAllocate, ok := explain["can_allocate"].(string); ok {
		parsed["can_allocate"] = canAllocate
		switch canAllocate {
		case "NO":
			parsed["allocate_explanation"] = "当前没有任何节点可以分配此分片"
		case "THROTTLED":
			parsed["allocate_explanation"] = "分片分配被限流，等待中"
		case "YES":
			parsed["allocate_explanation"] = "分片可以被分配"
		}
	}

	if allocateExplanation, ok := explain["allocate_explanation"].(string); ok {
		parsed["allocate_explanation_detail"] = allocateExplanation
	}

	if nodeDecisions, ok := explain["node_allocation_decisions"].([]interface{}); ok {
		reasonCounts := map[string]int{}
		reasonSampleNode := map[string]string{}
		totalNo, totalYes, totalThrottle := 0, 0, 0

		for _, nd := range nodeDecisions {
			ndMap, ok := nd.(map[string]interface{})
			if !ok {
				continue
			}
			nodeDecision, _ := ndMap["node_decision"].(string)
			nodeName, _ := ndMap["node_name"].(string)

			switch nodeDecision {
			case "no":
				totalNo++
			case "yes":
				totalYes++
			case "throttled":
				totalThrottle++
			}

			if deciders, ok := ndMap["deciders"].([]interface{}); ok {
				for _, d := range deciders {
					dMap, ok := d.(map[string]interface{})
					if !ok {
						continue
					}
					if dd, _ := dMap["decision"].(string); dd == "NO" {
						deciderName, _ := dMap["decider"].(string)
						explanationStr, _ := dMap["explanation"].(string)
						key := fmt.Sprintf("[%s] %s", deciderName, explanationStr)
						reasonCounts[key]++
						if _, exists := reasonSampleNode[key]; !exists {
							reasonSampleNode[key] = nodeName
						}
					}
				}
			}
		}

		parsed["node_decisions_summary"] = map[string]interface{}{
			"total_nodes":    len(nodeDecisions),
			"no_count":       totalNo,
			"yes_count":      totalYes,
			"throttle_count": totalThrottle,
		}

		if len(reasonCounts) > 0 {
			reasons := []map[string]interface{}{}
			for reason, count := range reasonCounts {
				reasons = append(reasons, map[string]interface{}{
					"reason":         reason,
					"affected_nodes": count,
					"sample_node":    reasonSampleNode[reason],
				})
			}
			if len(reasons) > 10 {
				parsed["reject_reasons"] = reasons[:10]
				parsed["reject_reasons_total_types"] = len(reasons)
			} else {
				parsed["reject_reasons"] = reasons
			}
		}
	}

	return parsed
}

// aggregateUnassignedFromHealth 从 cluster health indices 中提取未分配分片信息
func aggregateUnassignedFromHealth(health *elasticsearch.ClusterHealthDetailedResponse) []map[string]interface{} {
	result := []map[string]interface{}{}
	for indexName, idx := range health.Indices {
		if idx.UnassignedShards == 0 {
			continue
		}
		expectedTotal := idx.NumberOfShards * (1 + idx.NumberOfReplicas)
		missingPrimary := idx.NumberOfShards - idx.ActivePrimaryShards
		missingReplica := idx.UnassignedShards - missingPrimary

		entry := map[string]interface{}{
			"index":              indexName,
			"status":             idx.Status,
			"shards":             idx.NumberOfShards,
			"replicas":           idx.NumberOfReplicas,
			"expected_total":     expectedTotal,
			"active":             idx.ActiveShards,
			"unassigned_total":   idx.UnassignedShards,
			"unassigned_primary": missingPrimary,
			"unassigned_replica": missingReplica,
		}
		if missingPrimary > 0 {
			entry["severity"] = "RED - 主分片丢失，数据不可用"
		} else {
			entry["severity"] = "YELLOW - 仅副本分片缺失，数据可用但无冗余"
		}
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		iTotal, _ := result[i]["unassigned_total"].(int)
		jTotal, _ := result[j]["unassigned_total"].(int)
		iPri, _ := result[i]["unassigned_primary"].(int)
		jPri, _ := result[j]["unassigned_primary"].(int)
		if iPri != jPri {
			return iPri > jPri
		}
		return iTotal > jTotal
	})
	return result
}

// summarizeDiskWatermark 生成磁盘水位线统计摘要
func summarizeDiskWatermark(allocation []elasticsearch.AllocationInfo) map[string]interface{} {
	total := len(allocation)
	critical, high, warning, normal := 0, 0, 0, 0
	var maxPct float64
	var maxNode string

	for _, alloc := range allocation {
		if alloc.DiskPercent == "" {
			continue
		}
		var pct float64
		fmt.Sscanf(alloc.DiskPercent, "%f", &pct)

		if pct > maxPct {
			maxPct = pct
			maxNode = alloc.Node
		}

		if pct >= 95 {
			critical++
		} else if pct >= 85 {
			high++
		} else if pct >= 80 {
			warning++
		} else {
			normal++
		}
	}

	return map[string]interface{}{
		"total_nodes":            total,
		"normal_below_80":        normal,
		"warning_80_85":          warning,
		"high_85_95":             high,
		"critical_above_95":      critical,
		"max_disk_percent":       fmt.Sprintf("%.1f%%", maxPct),
		"max_disk_percent_node":  maxNode,
	}
}

// analyzeDiskWatermark 分析节点磁盘水位线
func analyzeDiskWatermark(allocation []elasticsearch.AllocationInfo) []map[string]interface{} {
	issues := []map[string]interface{}{}
	for _, alloc := range allocation {
		diskPercent := alloc.DiskPercent
		if diskPercent == "" {
			continue
		}
		var pct float64
		fmt.Sscanf(diskPercent, "%f", &pct)

		if pct >= 95 {
			issues = append(issues, map[string]interface{}{
				"node":         alloc.Node,
				"disk_percent": diskPercent,
				"disk_used":    alloc.DiskUsed,
				"disk_avail":   alloc.DiskAvail,
				"disk_total":   alloc.DiskTotal,
				"level":        "CRITICAL",
				"message":      fmt.Sprintf("节点 '%s' 磁盘使用率 %s%% 已超过 flood stage watermark (95%%)，索引将变为只读！", alloc.Node, diskPercent),
			})
		} else if pct >= 85 {
			issues = append(issues, map[string]interface{}{
				"node":         alloc.Node,
				"disk_percent": diskPercent,
				"disk_used":    alloc.DiskUsed,
				"disk_avail":   alloc.DiskAvail,
				"disk_total":   alloc.DiskTotal,
				"level":        "HIGH",
				"message":      fmt.Sprintf("节点 '%s' 磁盘使用率 %s%% 已超过 high watermark (85%%)，ES 正在迁移分片离开此节点", alloc.Node, diskPercent),
			})
		} else if pct >= 80 {
			issues = append(issues, map[string]interface{}{
				"node":         alloc.Node,
				"disk_percent": diskPercent,
				"disk_used":    alloc.DiskUsed,
				"disk_avail":   alloc.DiskAvail,
				"disk_total":   alloc.DiskTotal,
				"level":        "WARNING",
				"message":      fmt.Sprintf("节点 '%s' 磁盘使用率 %s%% 已超过 low watermark (80%%)，ES 将不再向此节点分配新分片", alloc.Node, diskPercent),
			})
		}
	}
	return issues
}
