// Package elasticsearch 定义 Elasticsearch 操作使用的类型和结构。
// 它包含响应类型、请求类型和 MCP 服务器的实用类型。
package elasticsearch

import (
	"io"
	"log"
	"net/http"
	"time"
)

// InfoResponse 表示 Elasticsearch 集群信息 API 的响应
type InfoResponse struct {
	Name        string `json:"name"`
	ClusterName string `json:"cluster_name"`
	ClusterUUID string `json:"cluster_uuid"`
	Version     struct {
		Number                           string `json:"number"`
		BuildFlavor                      string `json:"build_flavor"`
		BuildType                        string `json:"build_type"`
		BuildHash                        string `json:"build_hash"`
		BuildDate                        string `json:"build_date"`
		BuildSnapshot                    bool   `json:"build_snapshot"`
		LuceneVersion                    string `json:"lucene_version"`
		MinimumWireCompatibilityVersion  string `json:"minimum_wire_compatibility_version"`
		MinimumIndexCompatibilityVersion string `json:"minimum_index_compatibility_version"`
	} `json:"version"`
	TagLine string `json:"tagline"`
}

// HealthResponse 表示 Elasticsearch 集群健康 API 的响应
type HealthResponse struct {
	ClusterName                 string  `json:"cluster_name"`
	Status                      string  `json:"status"`
	TimedOut                    bool    `json:"timed_out"`
	NumberOfNodes               int     `json:"number_of_nodes"`
	NumberOfDataNodes           int     `json:"number_of_data_nodes"`
	ActivePrimaryShards         int     `json:"active_primary_shards"`
	ActiveShards                int     `json:"active_shards"`
	RelocatingShards            int     `json:"relocating_shards"`
	InitializingShards          int     `json:"initializing_shards"`
	UnassignedShards            int     `json:"unassigned_shards"`
	DelayedUnassignedShards     int     `json:"delayed_unassigned_shards"`
	NumberOfPendingTasks        int     `json:"number_of_pending_tasks"`
	NumberOfInFlightFetch       int     `json:"number_of_in_flight_fetch"`
	TaskMaxWaitingInQueueMillis int     `json:"task_max_waiting_in_queue_millis"`
	ActiveShardsPercentAsNumber float64 `json:"active_shards_percent_as_number"`
}

// IndexInfo 包含 Elasticsearch 索引的信息
type IndexInfo struct {
	Health       string `json:"health"`
	Status       string `json:"status"`
	Index        string `json:"index"`
	UUID         string `json:"uuid"`
	Pri          string `json:"pri"`
	Rep          string `json:"rep"`
	DocsCount    string `json:"docs.count"`
	DocsDeleted  string `json:"docs.deleted"`
	StoreSize    string `json:"store.size"`
	PriStoreSize string `json:"pri.store.size"`
}

// IndexResponse 表示文档索引操作的响应
type IndexResponse struct {
	Index   string `json:"_index"`
	Type    string `json:"_type"`
	ID      string `json:"_id"`
	Version int    `json:"_version"`
	Result  string `json:"result"`
	Shards  struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
	SeqNo       int `json:"_seq_no"`
	PrimaryTerm int `json:"_primary_term"`
}

// GetResponse 表示文档检索操作的响应
type GetResponse struct {
	Index   string                 `json:"_index"`
	Type    string                 `json:"_type"`
	ID      string                 `json:"_id"`
	Version int                    `json:"_version"`
	SeqNo   int                    `json:"_seq_no"`
	Found   bool                   `json:"found"`
	Source  map[string]interface{} `json:"_source"`
}

// SearchRequest 表示 Elasticsearch 的搜索请求
type SearchRequest struct {
	Index           string                 `json:"index,omitempty"`
	Query           map[string]interface{} `json:"query,omitempty"`
	Size            int                    `json:"size,omitempty"`
	From            int                    `json:"from,omitempty"`
	Sort            []interface{}          `json:"sort,omitempty"`
	Source          interface{}            `json:"_source,omitempty"`
	TrackTotalHits  *bool                  `json:"track_total_hits,omitempty"`
}

// SearchResponse 表示搜索操作的响应
type SearchResponse struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Skipped    int `json:"skipped"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
	Hits struct {
		Total struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		MaxScore float64     `json:"max_score"`
		Hits     []SearchHit `json:"hits"`
	} `json:"hits"`
}

// SearchHit 表示单个搜索结果
type SearchHit struct {
	Index  string                 `json:"_index"`
	Type   string                 `json:"_type"`
	ID     string                 `json:"_id"`
	Score  float64                `json:"_score"`
	Source map[string]interface{} `json:"_source"`
}

// BulkOperation 表示批量请求中的单个操作
type BulkOperation struct {
	Operation string                 `json:"operation"`
	Index     string                 `json:"index"`
	Type      string                 `json:"type,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Body      map[string]interface{} `json:"body,omitempty"`
}

// BulkResponse 表示批量操作的响应
type BulkResponse struct {
	Took   int                           `json:"took"`
	Errors bool                          `json:"errors"`
	Items  []map[string]BulkItemResponse `json:"items"`
}

// BulkItemResponse 表示批量操作中单个项目的响应
type BulkItemResponse struct {
	Index   string `json:"_index"`
	Type    string `json:"_type"`
	ID      string `json:"_id"`
	Version int    `json:"_version"`
	Result  string `json:"result"`
	Status  int    `json:"status"`
	Error   *struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
	} `json:"error,omitempty"`
}

// ClusterHealthDetailedResponse 表示集群健康状态的详细响应
type ClusterHealthDetailedResponse struct {
	ClusterName                 string                 `json:"cluster_name"`
	Status                      string                 `json:"status"`
	TimedOut                    bool                   `json:"timed_out"`
	NumberOfNodes               int                    `json:"number_of_nodes"`
	NumberOfDataNodes           int                    `json:"number_of_data_nodes"`
	ActivePrimaryShards         int                    `json:"active_primary_shards"`
	ActiveShards                int                    `json:"active_shards"`
	RelocatingShards            int                    `json:"relocating_shards"`
	InitializingShards          int                    `json:"initializing_shards"`
	UnassignedShards            int                    `json:"unassigned_shards"`
	DelayedUnassignedShards     int                    `json:"delayed_unassigned_shards"`
	NumberOfPendingTasks        int                    `json:"number_of_pending_tasks"`
	NumberOfInFlightFetch       int                    `json:"number_of_in_flight_fetch"`
	TaskMaxWaitingInQueueMillis int                    `json:"task_max_waiting_in_queue_millis"`
	ActiveShardsPercentAsNumber float64                `json:"active_shards_percent_as_number"`
	Indices                     map[string]IndexHealth `json:"indices,omitempty"`
}

// IndexHealth 表示单个索引的健康状态
type IndexHealth struct {
	Status              string `json:"status"`
	NumberOfShards      int    `json:"number_of_shards"`
	NumberOfReplicas    int    `json:"number_of_replicas"`
	ActivePrimaryShards int    `json:"active_primary_shards"`
	ActiveShards        int    `json:"active_shards"`
	RelocatingShards    int    `json:"relocating_shards"`
	InitializingShards  int    `json:"initializing_shards"`
	UnassignedShards    int    `json:"unassigned_shards"`
}

// bodyReader 为请求体实现 io.Reader 接口
type bodyReader struct {
	data []byte
	pos  int
}

func (br *bodyReader) Read(p []byte) (int, error) {
	if br.pos >= len(br.data) {
		return 0, io.EOF
	}

	n := copy(p, br.data[br.pos:])
	br.pos += n
	return n, nil
}

func (br *bodyReader) Close() error {
	return nil
}

// esLogger 为 Elasticsearch 客户端实现一个简单的日志记录器
type esLogger struct{}

// LogRoundTrip 记录 HTTP 请求/响应信息以进行调试
func (l *esLogger) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	if err != nil {
		log.Printf("Elasticsearch 请求失败: %v", err)
	} else {
		log.Printf("Elasticsearch %s %s %d %s", req.Method, req.URL.Path, res.StatusCode, dur)
	}
	return nil
}

// RequestBodyEnabled 返回 true 以启用请求体日志记录
func (l *esLogger) RequestBodyEnabled() bool {
	return false
}

// ResponseBodyEnabled 返回 true 以启用响应体日志记录
func (l *esLogger) ResponseBodyEnabled() bool {
	return false
}

// closeBody 是一个辅助函数，用于安全地关闭响应体
func closeBody(body io.Closer) {
	if err := body.Close(); err != nil {
		log.Printf("关闭响应体失败: %v", err)
	}
}

// ===== 新增类型 =====

// ClusterStatsResponse 表示集群统计信息的响应（通用 map，字段较多）
type ClusterStatsResponse = map[string]interface{}

// NodesInfoResponse 表示节点信息的响应
type NodesInfoResponse = map[string]interface{}

// NodesStatsResponse 表示节点统计信息的响应
type NodesStatsResponse = map[string]interface{}

// IndexDetailResponse 表示索引详情的响应（包含 mappings、settings、aliases）
type IndexDetailResponse = map[string]interface{}

// IndexStatsResponse 表示索引统计信息的响应
type IndexStatsResponse = map[string]interface{}

// CountResponse 表示文档计数的响应
type CountResponse struct {
	Count  int64 `json:"count"`
	Shards struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Skipped    int `json:"skipped"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
}

// ShardInfo 表示分片信息（cat shards）
type ShardInfo = map[string]interface{}

// SegmentInfo 表示段信息（cat segments）
type SegmentInfo = map[string]interface{}

// TasksResponse 表示任务列表的响应
type TasksResponse = map[string]interface{}

// TaskDetailResponse 表示任务详情的响应
type TaskDetailResponse = map[string]interface{}

// TemplateResponse 表示索引模板的响应
type TemplateResponse = map[string]interface{}

// RepositoryResponse 表示快照仓库的响应
type RepositoryResponse = map[string]interface{}

// SnapshotResponse 表示快照列表的响应
type SnapshotResponse = map[string]interface{}

// ILMPolicyResponse 表示 ILM 策略的响应
type ILMPolicyResponse = map[string]interface{}

// ILMStatusResponse 表示索引 ILM 状态的响应
type ILMStatusResponse = map[string]interface{}

// AliasResponse 表示别名信息的响应
type AliasResponse = map[string]interface{}

// AcknowledgedResponse 表示通用确认响应
type AcknowledgedResponse struct {
	Acknowledged       bool `json:"acknowledged"`
	ShardsAcknowledged bool `json:"shards_acknowledged,omitempty"`
	Index              string `json:"index,omitempty"`
}

// SearchRequestExtended 扩展搜索请求，支持聚合和高亮
type SearchRequestExtended struct {
	SearchRequest
	Aggs      map[string]interface{} `json:"aggs,omitempty"`
	Highlight map[string]interface{} `json:"highlight,omitempty"`
}

// MSearchItem 表示 Multi Search API 中的单个搜索请求
type MSearchItem struct {
	Index string                 `json:"index,omitempty"`
	Body  map[string]interface{} `json:"body"`
}

// SQLQueryResponse 表示 SQL 查询的响应
type SQLQueryResponse = map[string]interface{}

// PendingTaskInfo 表示待处理任务信息
type PendingTaskInfo struct {
	InsertOrder       int    `json:"insert_order"`
	Priority          string `json:"priority"`
	Source            string `json:"source"`
	TimeInQueueMillis int    `json:"time_in_queue_millis"`
	TimeInQueue       string `json:"time_in_queue"`
}

// RecoveryInfo 表示索引恢复信息
type RecoveryInfo = map[string]interface{}

// ThreadPoolInfo 表示线程池信息
type ThreadPoolInfo struct {
	NodeName string `json:"node_name"`
	Name     string `json:"name"`
	Active   string `json:"active"`
	Queue    string `json:"queue"`
	Rejected string `json:"rejected"`
	Type     string `json:"type"`
	Size     string `json:"size"`
}

// AllocationInfo 表示分片分配信息
type AllocationInfo struct {
	Shards      string `json:"shards"`
	DiskUsed    string `json:"disk.used"`
	DiskAvail   string `json:"disk.avail"`
	DiskTotal   string `json:"disk.total"`
	DiskPercent string `json:"disk.percent"`
	Host        string `json:"host"`
	IP          string `json:"ip"`
	Node        string `json:"node"`
}

// AllocationExplainResponse 表示分片分配解释的响应
type AllocationExplainResponse = map[string]interface{}

// HotThreadsResponse 表示热线程信息
type HotThreadsResponse struct {
	RawText string `json:"raw_text"`
}
