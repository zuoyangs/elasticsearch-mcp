// Package thanos 提供轻量级 Thanos/Prometheus Query API 客户端。
// 用于从 Thanos Query 获取 Elasticsearch 集群的 Prometheus 监控指标。
package thanos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client Thanos Query 客户端
type Client struct {
	Endpoint string
	Timeout  time.Duration
	client   *http.Client
}

// NewClient 创建 Thanos 客户端
func NewClient(endpoint string, timeout time.Duration) *Client {
	return &Client{
		Endpoint: strings.TrimRight(endpoint, "/"),
		Timeout:  timeout,
		client:   &http.Client{Timeout: timeout},
	}
}

// QueryResult 即时查询结果
type QueryResult struct {
	Status    string      `json:"status"`
	Data      *QueryData  `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	ErrorType string      `json:"errorType,omitempty"`
}

// QueryData 查询数据
type QueryData struct {
	ResultType string      `json:"resultType"`
	Result     interface{} `json:"result"`
}

// Query 执行即时 PromQL 查询
func (c *Client) Query(ctx context.Context, query string) (*QueryResult, error) {
	if c.Endpoint == "" {
		return nil, fmt.Errorf("Thanos endpoint 未配置")
	}

	u := fmt.Sprintf("%s/api/v1/query?query=%s", c.Endpoint, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Thanos 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		preview := string(body)
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		return nil, fmt.Errorf("Thanos 返回 HTTP %d: %s", resp.StatusCode, preview)
	}

	var result QueryResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &result, nil
}

// RangeQueryResult 范围查询结果
type RangeQueryResult struct {
	Status    string           `json:"status"`
	Data      *RangeQueryData  `json:"data,omitempty"`
	Error     string           `json:"error,omitempty"`
	ErrorType string           `json:"errorType,omitempty"`
}

// RangeQueryData 范围查询数据
type RangeQueryData struct {
	ResultType string      `json:"resultType"`
	Result     interface{} `json:"result"`
}

// RangeQuery 执行范围 PromQL 查询
func (c *Client) RangeQuery(ctx context.Context, query string, start, end int64, step string) (*RangeQueryResult, error) {
	if c.Endpoint == "" {
		return nil, fmt.Errorf("Thanos endpoint 未配置")
	}

	u := fmt.Sprintf("%s/api/v1/query_range?query=%s&start=%d&end=%d&step=%s",
		c.Endpoint, url.QueryEscape(query), start, end, url.QueryEscape(step))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Thanos 失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		preview := string(body)
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		return nil, fmt.Errorf("Thanos 返回 HTTP %d: %s", resp.StatusCode, preview)
	}

	var result RangeQueryResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	return &result, nil
}
