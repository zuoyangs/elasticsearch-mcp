// Package tools 提供大规模集群数据的智能摘要函数。
// 这些函数将原始的大量数据提炼为关键信息和异常点，
// 确保返回给 LLM 的数据量可控且信息密度高。
package tools

import (
	"fmt"
	"sort"
	"strconv"

	"elasticsearch-mcp/elasticsearch"
)

// ===== 索引列表摘要 =====

// summarizeIndices 对大量索引生成摘要：按健康状态聚合、存储 Top N、状态分布
func summarizeIndices(indices []elasticsearch.IndexInfo) map[string]interface{} {
	greenCount, yellowCount, redCount, closedCount := 0, 0, 0, 0
	var nonGreenIndices []elasticsearch.IndexInfo

	for _, idx := range indices {
		switch idx.Health {
		case "green":
			greenCount++
		case "yellow":
			yellowCount++
			nonGreenIndices = append(nonGreenIndices, idx)
		case "red":
			redCount++
			nonGreenIndices = append(nonGreenIndices, idx)
		}
		if idx.Status == "close" {
			closedCount++
		}
	}

	// 按存储大小排序取 Top 20（存储最大的索引）
	sorted := make([]elasticsearch.IndexInfo, len(indices))
	copy(sorted, indices)
	sort.Slice(sorted, func(i, j int) bool {
		return parseStorageSize(sorted[i].StoreSize) > parseStorageSize(sorted[j].StoreSize)
	})
	topBySize := sorted
	if len(topBySize) > 20 {
		topBySize = topBySize[:20]
	}

	result := map[string]interface{}{
		"total_count": len(indices),
		"health_distribution": map[string]int{
			"green":  greenCount,
			"yellow": yellowCount,
			"red":    redCount,
		},
		"closed_count":       closedCount,
		"top_20_by_size":     topBySize,
	}

	// 非 green 索引全部列出（通常不会太多），最多 50 个
	if len(nonGreenIndices) > 0 {
		if len(nonGreenIndices) > 50 {
			result["non_green_indices"] = nonGreenIndices[:50]
			result["non_green_total"] = len(nonGreenIndices)
		} else {
			result["non_green_indices"] = nonGreenIndices
		}
	}

	return result
}

// ===== 分片摘要 =====

// summarizeShards 对分片数据生成摘要：按状态聚合、节点分布不均衡分析、异常分片
func summarizeShards(shards []elasticsearch.ShardInfo) map[string]interface{} {
	// 按状态聚合
	stateCounts := map[string]int{}
	// 按节点统计分片数
	nodeShardCounts := map[string]int{}
	// 收集异常分片（UNASSIGNED / RELOCATING / INITIALIZING）
	var abnormalShards []map[string]interface{}

	for _, shard := range shards {
		state, _ := shard["state"].(string)
		node, _ := shard["node"].(string)
		stateCounts[state]++
		if node != "" {
			nodeShardCounts[node]++
		}

		if state != "STARTED" {
			info := map[string]interface{}{
				"index":  shard["index"],
				"shard":  shard["shard"],
				"prirep": shard["prirep"],
				"state":  state,
			}
			if reason, ok := shard["unassigned.reason"].(string); ok && reason != "" {
				info["unassigned_reason"] = reason
			}
			if node != "" {
				info["node"] = node
			}
			abnormalShards = append(abnormalShards, info)
		}
	}

	result := map[string]interface{}{
		"total_shards":     len(shards),
		"state_distribution": stateCounts,
	}

	// 节点分片分布不均衡分析
	if len(nodeShardCounts) > 0 {
		minShards, maxShards := int(^uint(0)>>1), 0
		var minNode, maxNode string
		totalAssigned := 0
		for node, count := range nodeShardCounts {
			totalAssigned += count
			if count < minShards {
				minShards = count
				minNode = node
			}
			if count > maxShards {
				maxShards = count
				maxNode = node
			}
		}
		avg := float64(totalAssigned) / float64(len(nodeShardCounts))
		result["node_distribution"] = map[string]interface{}{
			"nodes_with_shards": len(nodeShardCounts),
			"avg_shards":        fmt.Sprintf("%.1f", avg),
			"min_shards":        minShards,
			"min_node":          minNode,
			"max_shards":        maxShards,
			"max_node":          maxNode,
			"skew_ratio":        fmt.Sprintf("%.2f", float64(maxShards)/avg),
		}
	}

	// 异常分片（最多 30 条）
	if len(abnormalShards) > 0 {
		if len(abnormalShards) > 30 {
			result["abnormal_shards"] = abnormalShards[:30]
			result["abnormal_shards_total"] = len(abnormalShards)
		} else {
			result["abnormal_shards"] = abnormalShards
		}
	}

	return result
}

// ===== 段摘要 =====

// summarizeSegments 对段数据生成摘要：按索引聚合段数和大小
func summarizeSegments(segments []elasticsearch.SegmentInfo) map[string]interface{} {
	type indexSegStat struct {
		SegmentCount int
		DocsCount    int64
		DocsDeleted  int64
		SizeInBytes  int64
	}
	indexStats := map[string]*indexSegStat{}

	for _, seg := range segments {
		idx, _ := seg["index"].(string)
		if indexStats[idx] == nil {
			indexStats[idx] = &indexSegStat{}
		}
		stat := indexStats[idx]
		stat.SegmentCount++
		if dc, ok := seg["docs.count"].(string); ok {
			if v, err := strconv.ParseInt(dc, 10, 64); err == nil {
				stat.DocsCount += v
			}
		}
		if dd, ok := seg["docs.deleted"].(string); ok {
			if v, err := strconv.ParseInt(dd, 10, 64); err == nil {
				stat.DocsDeleted += v
			}
		}
		if sz, ok := seg["size"].(string); ok {
			stat.SizeInBytes += parseStorageSize(sz)
		}
	}

	// 按段数降序排列，找出段碎片化严重的索引
	type indexEntry struct {
		Index        string  `json:"index"`
		Segments     int     `json:"segments"`
		DocsCount    int64   `json:"docs_count"`
		DocsDeleted  int64   `json:"docs_deleted"`
		DeleteRatio  string  `json:"delete_ratio"`
	}
	var entries []map[string]interface{}
	for idx, stat := range indexStats {
		deleteRatio := float64(0)
		if stat.DocsCount+stat.DocsDeleted > 0 {
			deleteRatio = float64(stat.DocsDeleted) / float64(stat.DocsCount+stat.DocsDeleted) * 100
		}
		entries = append(entries, map[string]interface{}{
			"index":        idx,
			"segments":     stat.SegmentCount,
			"docs_count":   stat.DocsCount,
			"docs_deleted": stat.DocsDeleted,
			"delete_ratio": fmt.Sprintf("%.1f%%", deleteRatio),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i]["segments"].(int) > entries[j]["segments"].(int)
	})

	// 找出需要 force merge 的索引（删除文档比例 > 20% 或段数 > 50）
	var needsMerge []map[string]interface{}
	for _, e := range entries {
		segs := e["segments"].(int)
		deleted := e["docs_deleted"].(int64)
		total := e["docs_count"].(int64) + deleted
		ratio := float64(0)
		if total > 0 {
			ratio = float64(deleted) / float64(total) * 100
		}
		if segs > 50 || ratio > 20 {
			needsMerge = append(needsMerge, e)
		}
	}

	result := map[string]interface{}{
		"total_segments":  len(segments),
		"indices_count":   len(indexStats),
	}
	if len(entries) > 20 {
		result["top_20_by_segments"] = entries[:20]
	} else {
		result["by_index"] = entries
	}
	if len(needsMerge) > 0 {
		if len(needsMerge) > 20 {
			result["needs_force_merge"] = needsMerge[:20]
			result["needs_force_merge_total"] = len(needsMerge)
		} else {
			result["needs_force_merge"] = needsMerge
		}
	}

	return result
}

// ===== 分片分配摘要 =====

// summarizeAllocation 对节点分片分配数据生成摘要：磁盘水位线 + 分片分布不均衡
func summarizeAllocation(allocation []elasticsearch.AllocationInfo) map[string]interface{} {
	result := map[string]interface{}{
		"nodes_count":            len(allocation),
		"disk_watermark_summary": summarizeDiskWatermark(allocation),
	}

	// 分片分布不均衡分析
	if len(allocation) > 0 {
		minShards, maxShards := int(^uint(0)>>1), 0
		var minNode, maxNode string
		totalShards := 0
		nodeCount := 0

		for _, alloc := range allocation {
			if alloc.Shards == "" || alloc.Node == "" {
				continue
			}
			shardCount, err := strconv.Atoi(alloc.Shards)
			if err != nil {
				continue
			}
			nodeCount++
			totalShards += shardCount
			if shardCount < minShards {
				minShards = shardCount
				minNode = alloc.Node
			}
			if shardCount > maxShards {
				maxShards = shardCount
				maxNode = alloc.Node
			}
		}

		if nodeCount > 0 {
			avg := float64(totalShards) / float64(nodeCount)
			result["shard_balance"] = map[string]interface{}{
				"total_shards": totalShards,
				"avg_per_node": fmt.Sprintf("%.1f", avg),
				"min_shards":   minShards,
				"min_node":     minNode,
				"max_shards":   maxShards,
				"max_node":     maxNode,
				"skew_ratio":   fmt.Sprintf("%.2f", float64(maxShards)/avg),
			}
		}
	}

	// 只列出有磁盘问题的节点（>= 80%）
	var problemNodes []map[string]interface{}
	for _, alloc := range allocation {
		if alloc.DiskPercent == "" {
			continue
		}
		var pct float64
		fmt.Sscanf(alloc.DiskPercent, "%f", &pct)
		if pct >= 80 {
			level := "WARNING"
			if pct >= 95 {
				level = "CRITICAL"
			} else if pct >= 85 {
				level = "HIGH"
			}
			problemNodes = append(problemNodes, map[string]interface{}{
				"node":         alloc.Node,
				"shards":       alloc.Shards,
				"disk_percent": alloc.DiskPercent,
				"disk_used":    alloc.DiskUsed,
				"disk_avail":   alloc.DiskAvail,
				"disk_total":   alloc.DiskTotal,
				"level":        level,
			})
		}
	}
	if len(problemNodes) > 0 {
		sort.Slice(problemNodes, func(i, j int) bool {
			var pi, pj float64
			fmt.Sscanf(problemNodes[i]["disk_percent"].(string), "%f", &pi)
			fmt.Sscanf(problemNodes[j]["disk_percent"].(string), "%f", &pj)
			return pi > pj
		})
		if len(problemNodes) > 30 {
			result["disk_problem_nodes"] = problemNodes[:30]
			result["disk_problem_nodes_total"] = len(problemNodes)
		} else {
			result["disk_problem_nodes"] = problemNodes
		}
	}

	return result
}

// ===== 线程池摘要 =====

// summarizeThreadPools 提取有异常的线程池：rejected > 0 或 queue > 0
func summarizeThreadPools(pools []elasticsearch.ThreadPoolInfo) map[string]interface{} {
	totalRecords := len(pools)

	// 按线程池类型聚合
	type poolAgg struct {
		TotalActive   int
		TotalQueue    int
		TotalRejected int
		NodeCount     int
		// 有问题的节点
		ProblemNodes []map[string]interface{}
	}
	poolMap := map[string]*poolAgg{}

	for _, p := range pools {
		if poolMap[p.Name] == nil {
			poolMap[p.Name] = &poolAgg{}
		}
		agg := poolMap[p.Name]
		agg.NodeCount++

		active, _ := strconv.Atoi(p.Active)
		queue, _ := strconv.Atoi(p.Queue)
		rejected, _ := strconv.Atoi(p.Rejected)

		agg.TotalActive += active
		agg.TotalQueue += queue
		agg.TotalRejected += rejected

		// 只收集有问题的节点（rejected > 0 或 queue > 0）
		if rejected > 0 || queue > 0 {
			agg.ProblemNodes = append(agg.ProblemNodes, map[string]interface{}{
				"node":     p.NodeName,
				"active":   p.Active,
				"queue":    p.Queue,
				"rejected": p.Rejected,
			})
		}
	}

	// 构建摘要：只输出有异常的线程池
	var abnormalPools []map[string]interface{}
	var normalPoolNames []string

	for name, agg := range poolMap {
		if agg.TotalRejected > 0 || agg.TotalQueue > 0 {
			entry := map[string]interface{}{
				"pool_name":      name,
				"nodes":          agg.NodeCount,
				"total_active":   agg.TotalActive,
				"total_queue":    agg.TotalQueue,
				"total_rejected": agg.TotalRejected,
			}
			// 问题节点最多列 10 个
			if len(agg.ProblemNodes) > 10 {
				entry["problem_nodes"] = agg.ProblemNodes[:10]
				entry["problem_nodes_total"] = len(agg.ProblemNodes)
			} else {
				entry["problem_nodes"] = agg.ProblemNodes
			}
			abnormalPools = append(abnormalPools, entry)
		} else {
			normalPoolNames = append(normalPoolNames, name)
		}
	}

	// 按 rejected 数降序
	sort.Slice(abnormalPools, func(i, j int) bool {
		ri, _ := abnormalPools[i]["total_rejected"].(int)
		rj, _ := abnormalPools[j]["total_rejected"].(int)
		return ri > rj
	})

	result := map[string]interface{}{
		"total_records":    totalRecords,
		"pool_types_count": len(poolMap),
	}
	if len(abnormalPools) > 0 {
		result["abnormal_pools"] = abnormalPools
		result["abnormal_pools_count"] = len(abnormalPools)
	}
	if len(normalPoolNames) > 0 {
		result["normal_pools"] = normalPoolNames
	}
	if len(abnormalPools) == 0 {
		result["status"] = "所有线程池运行正常，无队列积压和拒绝"
	}

	return result
}

// ===== 待处理任务摘要 =====

// summarizePendingTasks 按优先级和来源聚合待处理任务
func summarizePendingTasks(tasks []elasticsearch.PendingTaskInfo) map[string]interface{} {
	if len(tasks) == 0 {
		return map[string]interface{}{
			"total":  0,
			"status": "无待处理任务，master 节点空闲",
		}
	}

	// 按优先级聚合
	priorityCounts := map[string]int{}
	// 按来源聚合
	sourceCounts := map[string]int{}
	maxWait := 0

	for _, t := range tasks {
		priorityCounts[t.Priority]++
		sourceCounts[t.Source]++
		if t.TimeInQueueMillis > maxWait {
			maxWait = t.TimeInQueueMillis
		}
	}

	// 来源按数量降序
	type sourceEntry struct {
		Source string `json:"source"`
		Count  int    `json:"count"`
	}
	var sources []map[string]interface{}
	for src, count := range sourceCounts {
		sources = append(sources, map[string]interface{}{
			"source": src,
			"count":  count,
		})
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i]["count"].(int) > sources[j]["count"].(int)
	})
	if len(sources) > 20 {
		sources = sources[:20]
	}

	result := map[string]interface{}{
		"total":               len(tasks),
		"by_priority":         priorityCounts,
		"top_sources":         sources,
		"max_wait_in_queue":   fmt.Sprintf("%dms", maxWait),
	}

	if len(tasks) > 100 {
		result["warning"] = fmt.Sprintf("待处理任务 %d 个，master 节点压力较大，可能导致集群状态更新延迟", len(tasks))
	}

	return result
}

// ===== 恢复信息摘要 =====

// summarizeRecovery 按恢复类型和阶段聚合恢复信息
func summarizeRecovery(recovery []elasticsearch.RecoveryInfo) map[string]interface{} {
	// 按类型聚合
	typeCounts := map[string]int{}
	// 按阶段聚合
	stageCounts := map[string]int{}
	// 找出未完成的恢复
	var inProgress []map[string]interface{}

	for _, r := range recovery {
		rType, _ := r["type"].(string)
		stage, _ := r["stage"].(string)
		typeCounts[rType]++
		stageCounts[stage]++

		if stage != "done" {
			entry := map[string]interface{}{
				"index":       r["index"],
				"shard":       r["shard"],
				"type":        rType,
				"stage":       stage,
				"source_node": r["source_node"],
				"target_node": r["target_node"],
			}
			if bytesRecovered, ok := r["bytes_recovered"].(string); ok {
				entry["bytes_recovered"] = bytesRecovered
			}
			if bytesPercent, ok := r["bytes_percent"].(string); ok {
				entry["bytes_percent"] = bytesPercent
			}
			inProgress = append(inProgress, entry)
		}
	}

	result := map[string]interface{}{
		"total_records":  len(recovery),
		"by_type":        typeCounts,
		"by_stage":       stageCounts,
	}

	if len(inProgress) > 0 {
		if len(inProgress) > 30 {
			result["in_progress"] = inProgress[:30]
			result["in_progress_total"] = len(inProgress)
		} else {
			result["in_progress"] = inProgress
		}
	} else {
		result["status"] = "所有分片恢复已完成"
	}

	return result
}

// ===== 辅助函数 =====

// parseStorageSize 将 ES 存储大小字符串（如 "10.5gb", "500mb", "1.2tb"）解析为字节数
func parseStorageSize(s string) int64 {
	if s == "" {
		return 0
	}
	s2 := s
	multiplier := int64(1)

	// 尝试匹配单位后缀
	for _, suffix := range []struct {
		unit string
		mult int64
	}{
		{"tb", 1024 * 1024 * 1024 * 1024},
		{"gb", 1024 * 1024 * 1024},
		{"mb", 1024 * 1024},
		{"kb", 1024},
		{"b", 1},
	} {
		if len(s2) > len(suffix.unit) {
			tail := s2[len(s2)-len(suffix.unit):]
			if tail == suffix.unit {
				s2 = s2[:len(s2)-len(suffix.unit)]
				multiplier = suffix.mult
				break
			}
		}
	}

	val, err := strconv.ParseFloat(s2, 64)
	if err != nil {
		return 0
	}
	return int64(val * float64(multiplier))
}
