# Elasticsearch MCP Server

基于 [MCP (Model Context Protocol)](https://modelcontextprotocol.io/) 的 Elasticsearch 查询与管理服务器，让 AI 助手能够直接与多个 Elasticsearch 集群交互。

## 功能特性

- 支持多数据源配置，可同时连接多个 Elasticsearch 集群
- 集群健康状态查询与分析（green/yellow/red 原因诊断）
- 索引管理（列表、检查、搜索）
- 文档查询（DSL 搜索、按 ID 获取）
- 三种传输模式：stdio、http（Streamable HTTP）、sse
- 支持 Basic Auth、SSL/TLS 连接
- Docker 多阶段构建 + k8s 部署支持

## MCP 工具

### 集群管理 (Cluster)

| 工具 | 说明 |
|------|------|
| foundation.elasticsearch.list-instances | 列出所有配置的数据源实例 |
| foundation.elasticsearch.query-cluster-info | 获取集群信息和版本详情 |
| foundation.elasticsearch.query-cluster-health | 获取集群健康状态和指标 |
| foundation.elasticsearch.analyze-cluster-health | 分析集群健康状态变化原因（green→yellow/red） |
| foundation.elasticsearch.query-cluster-stats | 获取集群统计信息 |
| foundation.elasticsearch.query-nodes-info | 获取节点信息 |
| foundation.elasticsearch.query-nodes-stats | 获取节点统计信息 |
| foundation.elasticsearch.resolve-node | 解析节点 ID 为 Pod 名称 |

### 索引管理 (Index)

| 工具 | 说明 |
|------|------|
| foundation.elasticsearch.list-indices | 列出所有索引及元数据 |
| foundation.elasticsearch.check-index-exists | 检查索引是否存在 |
| foundation.elasticsearch.query-index-detail | 获取索引详细信息 |
| foundation.elasticsearch.query-index-mapping | 获取索引 mapping |
| foundation.elasticsearch.query-index-settings | 获取索引配置 |
| foundation.elasticsearch.query-index-stats | 获取索引统计信息 |
| foundation.elasticsearch.create-index | 创建索引 |
| foundation.elasticsearch.open-index | 打开索引 |
| foundation.elasticsearch.close-index | 关闭索引 |
| foundation.elasticsearch.list-aliases | 列出所有别名 |
| foundation.elasticsearch.query-alias-detail | 查询别名详情 |

### 文档操作 (Document)

| 工具 | 说明 |
|------|------|
| foundation.elasticsearch.query-document | 按 ID 获取文档 |
| foundation.elasticsearch.count-documents | 统计文档数量 |
| foundation.elasticsearch.create-document | 创建/更新文档 |
| foundation.elasticsearch.search | 执行 DSL 搜索查询 |
| foundation.elasticsearch.query-sql | 执行 SQL 查询 |
| foundation.elasticsearch.translate-sql | SQL 转 DSL |

### 分片与存储 (Shards)

| 工具 | 说明 |
|------|------|
| foundation.elasticsearch.list-shards | 列出分片信息 |
| foundation.elasticsearch.list-segments | 列出段信息 |

### 任务管理 (Tasks)

| 工具 | 说明 |
|------|------|
| foundation.elasticsearch.list-tasks | 列出任务 |
| foundation.elasticsearch.query-task-detail | 查询任务详情 |

### 模板管理 (Templates)

| 工具 | 说明 |
|------|------|
| foundation.elasticsearch.list-templates | 列出索引模板 |
| foundation.elasticsearch.query-template-detail | 查询模板详情 |

### 快照与 ILM (Snapshot/ILM)

| 工具 | 说明 |
|------|------|
| foundation.elasticsearch.list-repositories | 列出快照仓库 |
| foundation.elasticsearch.list-snapshots | 列出快照 |
| foundation.elasticsearch.list-ilm-policies | 列出 ILM 策略 |
| foundation.elasticsearch.query-ilm-status | 查询 ILM 状态 |

### 运维监控 (Monitoring)

| 工具 | 说明 |
|------|------|
| foundation.elasticsearch.monitor-kpi-overview | KPI 总览 |
| foundation.elasticsearch.monitor-cluster-disk | 集群磁盘使用 |
| foundation.elasticsearch.monitor-cluster-ops | 集群操作统计 |
| foundation.elasticsearch.monitor-thread-pool | 线程池监控 |
| foundation.elasticsearch.monitor-nodes-cpu-memory | 节点 CPU/内存 |
| foundation.elasticsearch.monitor-nodes-gc | GC 监控 |
| foundation.elasticsearch.monitor-nodes-jvm-heap | JVM 堆内存 |
| foundation.elasticsearch.monitor-nodes-disk | 节点磁盘 |
| foundation.elasticsearch.monitor-breakers | 断路器状态 |
| foundation.elasticsearch.monitor-index-perf | 索引性能 |
| foundation.elasticsearch.monitor-index-cache | 索引缓存 |
| foundation.elasticsearch.monitor-nodes-network | 网络监控 |
| foundation.elasticsearch.monitor-segments | 段信息 |
| foundation.elasticsearch.monitor-nodes-latency | 延迟监控 |
| foundation.elasticsearch.monitor-cluster-indices | 集群索引统计 |
| foundation.elasticsearch.monitor-index-doc-count | 索引文档数 |
| foundation.elasticsearch.monitor-index-store-size | 索引存储大小 |
| foundation.elasticsearch.monitor-index-maintenance-ops | 索引维护操作 |
| foundation.elasticsearch.monitor-index-memory-details | 索引内存详情 |
| foundation.elasticsearch.monitor-index-shards | 索引分片 |
| foundation.elasticsearch.monitor-nodes-maintenance | 节点维护 |
| foundation.elasticsearch.monitor-nodes-jvm-heap-details | JVM 堆详情 |
| foundation.elasticsearch.monitor-nodes-topology | 节点拓扑 |
| foundation.elasticsearch.monitor-cost-overview | 成本概览 |
| foundation.elasticsearch.monitor-capacity-watermark | 容量水位线 |
| foundation.elasticsearch.monitor-index-storage-topn | 索引存储 TopN |
| foundation.elasticsearch.monitor-write-performance | 写入性能 |
| foundation.elasticsearch.monitor-search-performance | 搜索性能 |
| foundation.elasticsearch.monitor-node-balance | 节点负载均衡 |

### 日志发现 (Discover)

| 工具 | 说明 |
|------|------|
| foundation.elasticsearch.discover-logs | 发现日志 |
| foundation.elasticsearch.summarize-logs | 日志汇总 |
| foundation.elasticsearch.query-log-context | 日志上下文查询 |
| foundation.elasticsearch.query-field-stats | 字段统计 |
| foundation.elasticsearch.scroll-logs | 滚动查询日志 |

### 系统信息 (System)

| 工具 | 说明 |
|------|------|
| foundation.elasticsearch.list-allocation | 分片分配 |
| foundation.elasticsearch.list-thread-pools | 线程池列表 |
| foundation.elasticsearch.list-pending-tasks | 待处理任务 |
| foundation.elasticsearch.list-recovery | 恢复信息 |
| foundation.elasticsearch.query-hot-threads | 热线程信息 |

## 快速开始

### 配置文件

1. 复制示例配置文件：

```bash
cp etc/elasticsearch.yaml.example etc/elasticsearch.yaml
```

2. 编辑 `etc/elasticsearch.yaml`，填入真实值：

```yaml
datasources:
  prod-elk:
    addresses:
      - "http://es-node:9200"
    REDACTED_USERNAME: "elastic"
    REDACTED_PASSWORD: "your-REDACTED_PASSWORD"
    ssl: false
    timeout: "30s"
    max_retries: 3

default_datasource: "prod-elk"
elasticsearch_version: "7"

server:
  name: "Elasticsearch MCP Server"
  version: "1.0.0"
  protocol: "stdio"    # stdio | http | sse
  address: "0.0.0.0"
  port: 8080
```

### 运行

```bash
# 构建
go build -o elasticsearch-mcp .

# 启动（自动读取 etc/elasticsearch.yaml 或 etc/config.yaml）
./elasticsearch-mcp

# 指定配置文件路径启动（配置文件放在 etc/ 目录下）
# 注意：需要在项目根目录执行，确保 etc/elasticsearch.yaml 存在
CONFIG_FILE=etc/elasticsearch.yaml ./elasticsearch-mcp
```

### 环境变量（单数据源模式）

| 变量 | 说明 | 默认值 |
|------|------|--------|
| ES_ADDRESSES | Elasticsearch 地址（逗号分隔） | http://127.0.0.1:9200 |
| ES_USERNAME | 用户名 | — |
| ES_PASSWORD | 密码 | — |
| ES_SSL | 启用 SSL | false |
| ES_TIMEOUT | 请求超时 | 30s |
| MCP_PROTOCOL | 传输模式 | stdio |
| MCP_PORT | HTTP 端口 | 8080 |

## Docker

```bash
docker build -t elasticsearch-mcp .
docker run --rm -p 8080:8080 -v $(REDACTED_PWD)/etc:/app/etc elasticsearch-mcp
```

## License

MIT
