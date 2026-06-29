// Package elasticsearch 提供 Elasticsearch 客户端功能，支持多个版本。
// 它通过官方 Go 客户端库实现与 Elasticsearch 集群交互的全面接口。
package elasticsearch

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"elasticsearch-mcp/config"

	elasticsearch7 "github.com/elastic/go-elasticsearch/v7"
	esapi7 "github.com/elastic/go-elasticsearch/v7/esapi"
)

// Client 定义 Elasticsearch 操作的接口。
type Client interface {
	Ping(ctx context.Context) error
	Info(ctx context.Context) (*InfoResponse, error)
	Health(ctx context.Context) (*HealthResponse, error)
	GetDetailedHealth(ctx context.Context) (*ClusterHealthDetailedResponse, error)

	// 集群操作
	ClusterStats(ctx context.Context) (ClusterStatsResponse, error)
	NodesInfo(ctx context.Context, nodeID string) (NodesInfoResponse, error)
	NodesStats(ctx context.Context, nodeID string, metric string) (NodesStatsResponse, error)

	// 索引操作
	CreateIndex(ctx context.Context, index string, body map[string]interface{}) error
	IndexExists(ctx context.Context, index string) (bool, error)
	ListIndices(ctx context.Context) ([]IndexInfo, error)
	ListIndicesWithPattern(ctx context.Context, pattern string) ([]IndexInfo, error)
	GetIndex(ctx context.Context, index string) (IndexDetailResponse, error)
	GetMapping(ctx context.Context, index string) (map[string]interface{}, error)
	GetSettings(ctx context.Context, index string) (map[string]interface{}, error)
	IndexStats(ctx context.Context, index string) (IndexStatsResponse, error)
	OpenIndex(ctx context.Context, index string) error
	CloseIndex(ctx context.Context, index string) error

	// 文档操作
	Index(ctx context.Context, index, docID string, body map[string]interface{}) (*IndexResponse, error)
	Get(ctx context.Context, index, docID string) (*GetResponse, error)
	Update(ctx context.Context, index, docID string, body map[string]interface{}) error
	Count(ctx context.Context, index string, query map[string]interface{}) (*CountResponse, error)

	// 搜索操作
	Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error)
	SearchWithAggs(ctx context.Context, req *SearchRequestExtended) (*map[string]interface{}, error)
	SQLQuery(ctx context.Context, query string, format string) (interface{}, error)
	SQLTranslate(ctx context.Context, query string) (map[string]interface{}, error)

	// 批量操作
	Bulk(ctx context.Context, operations []BulkOperation) (*BulkResponse, error)

	// 别名操作
	ListAliases(ctx context.Context, index string) (AliasResponse, error)
	GetAlias(ctx context.Context, alias string) (AliasResponse, error)

	// Cat API
	CatShards(ctx context.Context, index string) ([]ShardInfo, error)
	CatSegments(ctx context.Context, index string) ([]SegmentInfo, error)
	CatThreadPool(ctx context.Context, threadPool string) ([]ThreadPoolInfo, error)
	CatAllocation(ctx context.Context) ([]AllocationInfo, error)
	CatPendingTasks(ctx context.Context) ([]PendingTaskInfo, error)
	CatRecovery(ctx context.Context, index string) ([]RecoveryInfo, error)

	// 任务操作
	ListTasks(ctx context.Context, detailed bool, actions string) (TasksResponse, error)
	GetTask(ctx context.Context, taskID string) (TaskDetailResponse, error)

	// 模板操作
	ListTemplates(ctx context.Context) ([]map[string]interface{}, error)
	GetTemplate(ctx context.Context, name string) (TemplateResponse, error)

	// 快照操作
	ListRepositories(ctx context.Context) (RepositoryResponse, error)
	ListSnapshots(ctx context.Context, repository string) (SnapshotResponse, error)

	// ILM 操作
	ListILMPolicies(ctx context.Context) (ILMPolicyResponse, error)
	GetILMStatus(ctx context.Context, index string) (ILMStatusResponse, error)

	// 诊断操作
	NodesHotThreads(ctx context.Context, nodeID string) (string, error)
	ClusterAllocationExplain(ctx context.Context, index string, shard int, primary bool) (AllocationExplainResponse, error)

	// Discover 日志查询操作
	FieldCaps(ctx context.Context, index string) (map[string]interface{}, error)
	MSearch(ctx context.Context, requests []MSearchItem) ([]map[string]interface{}, error)

	Close() error
}

// ESClient 使用官方 Elasticsearch Go 客户端实现 Client 接口。
type ESClient struct {
	client  *elasticsearch7.Client
	config  *config.ElasticsearchConfig
	version string
}

// NewClient 使用提供的配置创建新的 Elasticsearch 客户端。
func NewClient(cfg *config.ElasticsearchConfig, version string) (Client, error) {
	if version != "7" {
		return nil, fmt.Errorf("当前版本仅支持 Elasticsearch 7.x，指定的版本: %s", version)
	}

	esConfig := elasticsearch7.Config{
		Addresses: cfg.Addresses,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.InsecureSkipVerify,
			},
		},
		MaxRetries:    cfg.MaxRetries,
		RetryOnStatus: []int{502, 503, 504, 429},
		Logger:        &esLogger{},
	}

	if cfg.Username != "" && cfg.Password != "" {
		esConfig.Username = cfg.Username
		esConfig.Password = cfg.Password
	}

	client, err := elasticsearch7.NewClient(esConfig)
	if err != nil {
		return nil, fmt.Errorf("创建 Elasticsearch 客户端失败: %w", err)
	}

	esClient := &ESClient{client: client, config: cfg, version: version}

	if cfg.SkipConnectionTest {
		log.Printf("已跳过数据源 '%s' 的连接测试（skip_connection_test=true）", cfg.Addresses)
		return esClient, nil
	}

	if err := esClient.testConnection(); err != nil {
		return nil, fmt.Errorf("连接测试失败: %w", err)
	}

	return esClient, nil
}

func (c *ESClient) testConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := c.client.Cluster.Health(c.client.Cluster.Health.WithContext(ctx))
	if err == nil {
		return nil
	}
	if err.Error() == "the client noticed that the server is not Elasticsearch and we do not support this unknown product" {
		return nil
	}

	_, err = c.client.Info(c.client.Info.WithContext(ctx))
	if err == nil {
		return nil
	}
	if err.Error() == "the client noticed that the server is not Elasticsearch and we do not support this unknown product" {
		return nil
	}
	return err
}

func (c *ESClient) Ping(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := c.client.Cluster.Health(
		c.client.Cluster.Health.WithContext(checkCtx),
		c.client.Cluster.Health.WithTimeout(5*time.Second),
	)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "not Elasticsearch") {
		return nil
	}

	_, err = c.client.Info(c.client.Info.WithContext(checkCtx))
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "not Elasticsearch") {
		return nil
	}
	return err
}

func (c *ESClient) Info(ctx context.Context) (*InfoResponse, error) {
	res, err := c.client.Info(c.client.Info.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("获取集群信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %v", res)
	}
	var info InfoResponse
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &info, nil
}

func (c *ESClient) Health(ctx context.Context) (*HealthResponse, error) {
	res, err := c.client.Cluster.Health(c.client.Cluster.Health.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("获取集群健康状态失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %v", res)
	}
	var health HealthResponse
	if err := json.NewDecoder(res.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &health, nil
}

func (c *ESClient) GetDetailedHealth(ctx context.Context) (*ClusterHealthDetailedResponse, error) {
	res, err := c.client.Cluster.Health(
		c.client.Cluster.Health.WithContext(ctx),
		c.client.Cluster.Health.WithLevel("indices"),
	)
	if err != nil {
		return nil, fmt.Errorf("获取集群健康状态失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %v", res)
	}
	var health ClusterHealthDetailedResponse
	if err := json.NewDecoder(res.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &health, nil
}

// ===== 集群操作 =====

func (c *ESClient) ClusterStats(ctx context.Context) (ClusterStatsResponse, error) {
	res, err := c.client.Cluster.Stats(c.client.Cluster.Stats.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("获取集群统计信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result ClusterStatsResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) NodesInfo(ctx context.Context, nodeID string) (NodesInfoResponse, error) {
	opts := []func(*esapi7.NodesInfoRequest){
		c.client.Nodes.Info.WithContext(ctx),
	}
	if nodeID != "" {
		opts = append(opts, c.client.Nodes.Info.WithNodeID(nodeID))
	}
	res, err := c.client.Nodes.Info(opts...)
	if err != nil {
		return nil, fmt.Errorf("获取节点信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result NodesInfoResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) NodesStats(ctx context.Context, nodeID string, metric string) (NodesStatsResponse, error) {
	opts := []func(*esapi7.NodesStatsRequest){
		c.client.Nodes.Stats.WithContext(ctx),
	}
	if nodeID != "" {
		opts = append(opts, c.client.Nodes.Stats.WithNodeID(nodeID))
	}
	if metric != "" {
		opts = append(opts, c.client.Nodes.Stats.WithMetric(metric))
	}
	res, err := c.client.Nodes.Stats(opts...)
	if err != nil {
		return nil, fmt.Errorf("获取节点统计信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result NodesStatsResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// ===== 索引操作 =====

func (c *ESClient) CreateIndex(ctx context.Context, index string, body map[string]interface{}) error {
	req := esapi7.IndicesCreateRequest{Index: index}
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("序列化请求体失败: %w", err)
		}
		req.Body = &bodyReader{data: bodyBytes}
	}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return fmt.Errorf("创建索引失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	return nil
}

func (c *ESClient) IndexExists(ctx context.Context, index string) (bool, error) {
	req := esapi7.IndicesExistsRequest{Index: []string{index}}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return false, fmt.Errorf("检查索引存在性失败: %w", err)
	}
	defer res.Body.Close()
	return res.StatusCode == 200, nil
}

func (c *ESClient) ListIndices(ctx context.Context) ([]IndexInfo, error) {
	return c.ListIndicesWithPattern(ctx, "")
}

func (c *ESClient) ListIndicesWithPattern(ctx context.Context, pattern string) ([]IndexInfo, error) {
	req := esapi7.CatIndicesRequest{Format: "json"}
	if pattern != "" {
		req.Index = []string{pattern}
	}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("列出索引失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var indices []IndexInfo
	if err := json.NewDecoder(res.Body).Decode(&indices); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return indices, nil
}

func (c *ESClient) GetIndex(ctx context.Context, index string) (IndexDetailResponse, error) {
	req := esapi7.IndicesGetRequest{Index: []string{index}}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("获取索引详情失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result IndexDetailResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) GetMapping(ctx context.Context, index string) (map[string]interface{}, error) {
	req := esapi7.IndicesGetMappingRequest{Index: []string{index}}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("获取索引映射失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) GetSettings(ctx context.Context, index string) (map[string]interface{}, error) {
	req := esapi7.IndicesGetSettingsRequest{Index: []string{index}}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("获取索引设置失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) IndexStats(ctx context.Context, index string) (IndexStatsResponse, error) {
	opts := []func(*esapi7.IndicesStatsRequest){
		c.client.Indices.Stats.WithContext(ctx),
	}
	if index != "" {
		opts = append(opts, c.client.Indices.Stats.WithIndex(index))
	}
	res, err := c.client.Indices.Stats(opts...)
	if err != nil {
		return nil, fmt.Errorf("获取索引统计信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result IndexStatsResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) OpenIndex(ctx context.Context, index string) error {
	req := esapi7.IndicesOpenRequest{Index: []string{index}}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return fmt.Errorf("打开索引失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	return nil
}

func (c *ESClient) CloseIndex(ctx context.Context, index string) error {
	req := esapi7.IndicesCloseRequest{Index: []string{index}}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return fmt.Errorf("关闭索引失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	return nil
}

// ===== 文档操作 =====

func (c *ESClient) Index(ctx context.Context, index, docID string, body map[string]interface{}) (*IndexResponse, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化文档失败: %w", err)
	}
	req := esapi7.IndexRequest{
		Index:      index,
		DocumentID: docID,
		Body:       &bodyReader{data: bodyBytes},
		Refresh:    "true",
	}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("索引文档失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var indexResp IndexResponse
	if err := json.NewDecoder(res.Body).Decode(&indexResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &indexResp, nil
}

func (c *ESClient) Get(ctx context.Context, index, docID string) (*GetResponse, error) {
	req := esapi7.GetRequest{Index: index, DocumentID: docID}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("获取文档失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		if res.StatusCode == 404 {
			return nil, fmt.Errorf("文档未找到")
		}
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var getResp GetResponse
	if err := json.NewDecoder(res.Body).Decode(&getResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &getResp, nil
}

func (c *ESClient) Update(ctx context.Context, index, docID string, body map[string]interface{}) error {
	updateBody := map[string]interface{}{"doc": body}
	bodyBytes, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("序列化更新体失败: %w", err)
	}
	req := esapi7.UpdateRequest{
		Index:      index,
		DocumentID: docID,
		Body:       &bodyReader{data: bodyBytes},
		Refresh:    "true",
	}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return fmt.Errorf("更新文档失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	return nil
}

func (c *ESClient) Count(ctx context.Context, index string, query map[string]interface{}) (*CountResponse, error) {
	opts := []func(*esapi7.CountRequest){
		c.client.Count.WithContext(ctx),
	}
	if index != "" {
		opts = append(opts, c.client.Count.WithIndex(index))
	}
	if query != nil {
		bodyBytes, err := json.Marshal(map[string]interface{}{"query": query})
		if err != nil {
			return nil, fmt.Errorf("序列化查询失败: %w", err)
		}
		opts = append(opts, c.client.Count.WithBody(&bodyReader{data: bodyBytes}))
	}
	res, err := c.client.Count(opts...)
	if err != nil {
		return nil, fmt.Errorf("统计文档数量失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var countResp CountResponse
	if err := json.NewDecoder(res.Body).Decode(&countResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &countResp, nil
}

// ===== 搜索操作 =====

func (c *ESClient) Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	searchBody := make(map[string]interface{})
	if req.Query != nil {
		searchBody["query"] = req.Query
	}
	if len(req.Sort) > 0 {
		searchBody["sort"] = req.Sort
	}
	if req.Source != nil {
		searchBody["_source"] = req.Source
	}
	// 支持 track_total_hits 参数
	if req.TrackTotalHits != nil {
		searchBody["track_total_hits"] = *req.TrackTotalHits
	}
	bodyBytes, err := json.Marshal(searchBody)
	if err != nil {
		return nil, fmt.Errorf("序列化搜索请求失败: %w", err)
	}
	log.Printf("Elasticsearch 搜索请求体: %s", string(bodyBytes))

	var indices []string
	if req.Index != "" {
		indices = []string{req.Index}
	}
	esReq := esapi7.SearchRequest{
		Index: indices,
		Body:  &bodyReader{data: bodyBytes},
		Size:  &req.Size,
		From:  &req.From,
	}
	res, err := esReq.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("搜索失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var searchResp SearchResponse
	if err := json.NewDecoder(res.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("解析搜索响应失败: %w", err)
	}
	return &searchResp, nil
}

func (c *ESClient) SearchWithAggs(ctx context.Context, req *SearchRequestExtended) (*map[string]interface{}, error) {
	searchBody := make(map[string]interface{})
	if req.Query != nil {
		searchBody["query"] = req.Query
	}
	if len(req.Sort) > 0 {
		searchBody["sort"] = req.Sort
	}
	if req.Source != nil {
		searchBody["_source"] = req.Source
	}
	if req.Aggs != nil {
		searchBody["aggs"] = req.Aggs
	}
	if req.Highlight != nil {
		searchBody["highlight"] = req.Highlight
	}
	// 支持 track_total_hits 参数
	if req.TrackTotalHits != nil {
		searchBody["track_total_hits"] = *req.TrackTotalHits
	}
	bodyBytes, err := json.Marshal(searchBody)
	if err != nil {
		return nil, fmt.Errorf("序列化搜索请求失败: %w", err)
	}
	var indices []string
	if req.Index != "" {
		indices = []string{req.Index}
	}
	esReq := esapi7.SearchRequest{
		Index: indices,
		Body:  &bodyReader{data: bodyBytes},
		Size:  &req.Size,
		From:  &req.From,
	}
	res, err := esReq.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("搜索失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析搜索响应失败: %w", err)
	}
	return &result, nil
}

func (c *ESClient) SQLQuery(ctx context.Context, query string, format string) (interface{}, error) {
	body := map[string]interface{}{"query": query}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化 SQL 查询失败: %w", err)
	}
	req := esapi7.SQLQueryRequest{
		Body: &bodyReader{data: bodyBytes},
	}
	if format != "" {
		req.Format = format
	}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("执行 SQL 查询失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	if format == "csv" || format == "txt" || format == "tsv" {
		data, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %w", err)
		}
		return string(data), nil
	}
	var result interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) SQLTranslate(ctx context.Context, query string) (map[string]interface{}, error) {
	body := map[string]interface{}{"query": query}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("序列化 SQL 查询失败: %w", err)
	}
	req := esapi7.SQLTranslateRequest{
		Body: &bodyReader{data: bodyBytes},
	}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("翻译 SQL 查询失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// ===== 批量操作 =====

func (c *ESClient) Bulk(ctx context.Context, operations []BulkOperation) (*BulkResponse, error) {
	body := ""
	for _, op := range operations {
		action := map[string]interface{}{
			op.Operation: map[string]interface{}{"_index": op.Index},
		}
		if op.Type != "" {
			action[op.Operation].(map[string]interface{})["_type"] = op.Type
		}
		if op.ID != "" {
			action[op.Operation].(map[string]interface{})["_id"] = op.ID
		}
		actionBytes, err := json.Marshal(action)
		if err != nil {
			return nil, fmt.Errorf("序列化批量操作失败: %w", err)
		}
		body += string(actionBytes) + "\n"
		if op.Body != nil {
			sourceBytes, err := json.Marshal(op.Body)
			if err != nil {
				return nil, fmt.Errorf("序列化批量操作源失败: %w", err)
			}
			body += string(sourceBytes) + "\n"
		}
	}
	req := esapi7.BulkRequest{Body: &bodyReader{data: []byte(body)}, Refresh: "true"}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("批量操作失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var bulkResp BulkResponse
	if err := json.NewDecoder(res.Body).Decode(&bulkResp); err != nil {
		return nil, fmt.Errorf("解析批量响应失败: %w", err)
	}
	return &bulkResp, nil
}

// ===== 别名操作 =====

func (c *ESClient) ListAliases(ctx context.Context, index string) (AliasResponse, error) {
	opts := []func(*esapi7.CatAliasesRequest){
		c.client.Cat.Aliases.WithContext(ctx),
		c.client.Cat.Aliases.WithFormat("json"),
	}
	if index != "" {
		opts = append(opts, c.client.Cat.Aliases.WithName(index))
	}
	res, err := c.client.Cat.Aliases(opts...)
	if err != nil {
		return nil, fmt.Errorf("列出别名失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return AliasResponse{"aliases": result}, nil
}

func (c *ESClient) GetAlias(ctx context.Context, alias string) (AliasResponse, error) {
	req := esapi7.IndicesGetAliasRequest{Name: []string{alias}}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("获取别名详情失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result AliasResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// ===== Cat API =====

func (c *ESClient) CatShards(ctx context.Context, index string) ([]ShardInfo, error) {
	opts := []func(*esapi7.CatShardsRequest){
		c.client.Cat.Shards.WithContext(ctx),
		c.client.Cat.Shards.WithFormat("json"),
		c.client.Cat.Shards.WithV(true),
	}
	if index != "" {
		opts = append(opts, c.client.Cat.Shards.WithIndex(index))
	}
	res, err := c.client.Cat.Shards(opts...)
	if err != nil {
		return nil, fmt.Errorf("获取分片信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result []ShardInfo
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) CatSegments(ctx context.Context, index string) ([]SegmentInfo, error) {
	opts := []func(*esapi7.CatSegmentsRequest){
		c.client.Cat.Segments.WithContext(ctx),
		c.client.Cat.Segments.WithFormat("json"),
		c.client.Cat.Segments.WithV(true),
	}
	if index != "" {
		opts = append(opts, c.client.Cat.Segments.WithIndex(index))
	}
	res, err := c.client.Cat.Segments(opts...)
	if err != nil {
		return nil, fmt.Errorf("获取段信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result []SegmentInfo
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) CatThreadPool(ctx context.Context, threadPool string) ([]ThreadPoolInfo, error) {
	opts := []func(*esapi7.CatThreadPoolRequest){
		c.client.Cat.ThreadPool.WithContext(ctx),
		c.client.Cat.ThreadPool.WithFormat("json"),
		c.client.Cat.ThreadPool.WithV(true),
	}
	if threadPool != "" {
		opts = append(opts, c.client.Cat.ThreadPool.WithThreadPoolPatterns(threadPool))
	}
	res, err := c.client.Cat.ThreadPool(opts...)
	if err != nil {
		return nil, fmt.Errorf("获取线程池信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result []ThreadPoolInfo
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) CatAllocation(ctx context.Context) ([]AllocationInfo, error) {
	res, err := c.client.Cat.Allocation(
		c.client.Cat.Allocation.WithContext(ctx),
		c.client.Cat.Allocation.WithFormat("json"),
		c.client.Cat.Allocation.WithV(true),
	)
	if err != nil {
		return nil, fmt.Errorf("获取分片分配信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result []AllocationInfo
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) CatPendingTasks(ctx context.Context) ([]PendingTaskInfo, error) {
	res, err := c.client.Cat.PendingTasks(
		c.client.Cat.PendingTasks.WithContext(ctx),
		c.client.Cat.PendingTasks.WithFormat("json"),
		c.client.Cat.PendingTasks.WithV(true),
	)
	if err != nil {
		return nil, fmt.Errorf("获取待处理任务失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result []PendingTaskInfo
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) CatRecovery(ctx context.Context, index string) ([]RecoveryInfo, error) {
	opts := []func(*esapi7.CatRecoveryRequest){
		c.client.Cat.Recovery.WithContext(ctx),
		c.client.Cat.Recovery.WithFormat("json"),
		c.client.Cat.Recovery.WithV(true),
	}
	if index != "" {
		opts = append(opts, c.client.Cat.Recovery.WithIndex(index))
	}
	res, err := c.client.Cat.Recovery(opts...)
	if err != nil {
		return nil, fmt.Errorf("获取恢复信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result []RecoveryInfo
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// ===== 任务操作 =====

func (c *ESClient) ListTasks(ctx context.Context, detailed bool, actions string) (TasksResponse, error) {
	opts := []func(*esapi7.TasksListRequest){
		c.client.Tasks.List.WithContext(ctx),
		c.client.Tasks.List.WithDetailed(detailed),
	}
	if actions != "" {
		opts = append(opts, c.client.Tasks.List.WithActions(actions))
	}
	res, err := c.client.Tasks.List(opts...)
	if err != nil {
		return nil, fmt.Errorf("列出任务失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result TasksResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) GetTask(ctx context.Context, taskID string) (TaskDetailResponse, error) {
	res, err := c.client.Tasks.Get(
		taskID,
		c.client.Tasks.Get.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("获取任务详情失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result TaskDetailResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// ===== 模板操作 =====

func (c *ESClient) ListTemplates(ctx context.Context) ([]map[string]interface{}, error) {
	res, err := c.client.Cat.Templates(
		c.client.Cat.Templates.WithContext(ctx),
		c.client.Cat.Templates.WithFormat("json"),
		c.client.Cat.Templates.WithV(true),
	)
	if err != nil {
		return nil, fmt.Errorf("列出模板失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result []map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) GetTemplate(ctx context.Context, name string) (TemplateResponse, error) {
	req := esapi7.IndicesGetTemplateRequest{Name: []string{name}}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("获取模板详情失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result TemplateResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// ===== 快照操作 =====

func (c *ESClient) ListRepositories(ctx context.Context) (RepositoryResponse, error) {
	res, err := c.client.Snapshot.GetRepository(
		c.client.Snapshot.GetRepository.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("列出快照仓库失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result RepositoryResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) ListSnapshots(ctx context.Context, repository string) (SnapshotResponse, error) {
	res, err := c.client.Snapshot.Get(
		repository,
		[]string{"*"},
		c.client.Snapshot.Get.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("列出快照失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result SnapshotResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// ===== ILM 操作 =====

func (c *ESClient) ListILMPolicies(ctx context.Context) (ILMPolicyResponse, error) {
	req := esapi7.ILMGetLifecycleRequest{}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("列出 ILM 策略失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result ILMPolicyResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

func (c *ESClient) GetILMStatus(ctx context.Context, index string) (ILMStatusResponse, error) {
	req := esapi7.ILMExplainLifecycleRequest{Index: index}
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, fmt.Errorf("获取 ILM 状态失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result ILMStatusResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// ===== 诊断操作 =====

func (c *ESClient) NodesHotThreads(ctx context.Context, nodeID string) (string, error) {
	opts := []func(*esapi7.NodesHotThreadsRequest){
		c.client.Nodes.HotThreads.WithContext(ctx),
	}
	if nodeID != "" {
		opts = append(opts, c.client.Nodes.HotThreads.WithNodeID(nodeID))
	}
	res, err := c.client.Nodes.HotThreads(opts...)
	if err != nil {
		return "", fmt.Errorf("获取热线程信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return "", fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	return string(data), nil
}

// ClusterAllocationExplain 调用 _cluster/allocation/explain API，获取分片未分配的详细原因。
// 如果 index 为空，ES 会自动选择第一个未分配的分片进行解释。
func (c *ESClient) ClusterAllocationExplain(ctx context.Context, index string, shard int, primary bool) (AllocationExplainResponse, error) {
	opts := []func(*esapi7.ClusterAllocationExplainRequest){
		c.client.Cluster.AllocationExplain.WithContext(ctx),
	}
	// 如果指定了 index，构建请求体
	if index != "" {
		body := map[string]interface{}{
			"index":   index,
			"shard":   shard,
			"primary": primary,
		}
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求体失败: %w", err)
		}
		opts = append(opts, c.client.Cluster.AllocationExplain.WithBody(&bodyReader{data: bodyBytes}))
	}
	res, err := c.client.Cluster.AllocationExplain(opts...)
	if err != nil {
		return nil, fmt.Errorf("获取分片分配解释失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		// 404 表示没有未分配的分片，这不是错误
		if res.StatusCode == 400 {
			return map[string]interface{}{"message": "没有未分配的分片需要解释"}, nil
		}
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result AllocationExplainResponse
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// Close 优雅地关闭 Elasticsearch 客户端连接。
func (c *ESClient) Close() error {
	return nil
}

// ===== Discover 日志查询操作 =====

// FieldCaps 获取索引的字段能力信息（字段名、类型、是否可聚合/搜索）
func (c *ESClient) FieldCaps(ctx context.Context, index string) (map[string]interface{}, error) {
	opts := []func(*esapi7.FieldCapsRequest){
		c.client.FieldCaps.WithContext(ctx),
		c.client.FieldCaps.WithFields("*"),
	}
	if index != "" {
		opts = append(opts, c.client.FieldCaps.WithIndex(index))
	}
	res, err := c.client.FieldCaps(opts...)
	if err != nil {
		return nil, fmt.Errorf("获取字段能力信息失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return result, nil
}

// MSearch 执行多搜索请求（Multi Search API），一次请求发送多个搜索
func (c *ESClient) MSearch(ctx context.Context, requests []MSearchItem) ([]map[string]interface{}, error) {
	var body strings.Builder
	for _, req := range requests {
		// header line
		header := make(map[string]interface{})
		if req.Index != "" {
			header["index"] = req.Index
		}
		headerBytes, err := json.Marshal(header)
		if err != nil {
			return nil, fmt.Errorf("序列化 msearch header 失败: %w", err)
		}
		body.Write(headerBytes)
		body.WriteByte('\n')

		// body line
		bodyBytes, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("序列化 msearch body 失败: %w", err)
		}
		body.Write(bodyBytes)
		body.WriteByte('\n')
	}

	res, err := c.client.Msearch(
		&bodyReader{data: []byte(body.String())},
		c.client.Msearch.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("多搜索请求失败: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch 错误: %s", res.String())
	}

	var msearchResp struct {
		Responses []map[string]interface{} `json:"responses"`
	}
	if err := json.NewDecoder(res.Body).Decode(&msearchResp); err != nil {
		return nil, fmt.Errorf("解析多搜索响应失败: %w", err)
	}
	return msearchResp.Responses, nil
}
