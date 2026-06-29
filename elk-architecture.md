# ELK 日志平台架构拓扑

## 整体架构

日志从 k8s 容器 → Fluent-Bit 采集 → Kafka 中转 → Logstash 消费 → Elasticsearch 存储。
跨机房日志通过 Kafka MirrorMaker / kafka_sync 管道同步。

## 数据中心分布

### 1. 阿里云（Aliyun）
- k8s 集群: <your-ack-cluster>
- Fluent-Bit: 采集容器日志，写入阿里云 Kafka
- Kafka: aliyun-kafka（<your-kafka-broker>:9093 等）
  - 云上日志先写入本地 Kafka
  - 通过 kafka_sync 同步到主 IDC 的 Kafka

### 2. 腾讯云（Tencent）
- k8s 集群: <your-tke-cluster>
- Fluent-Bit: 采集容器日志，写入腾讯云 CKafka
- Kafka: tencent-ckafka（<your-ckafka-broker>:9092）
  - 云上日志先写入本地 CKafka
  - 通过 kafka_sync 同步到主 IDC 的 Kafka

### 3. 主数据中心（Primary IDC）
- k8s 集群: <your-primary-cluster>
- Fluent-Bit: 采集本地 k8s 集群日志
- Kafka:
  - <primary-kafka>: 主日志 Kafka，接收云上同步过来的日志 + 本地日志
- Logstash:
  - 从主 Kafka 消费，写入主 ES
  - 部分管道做跨机房 Kafka 同步（kafka_sync pipeline）
- Elasticsearch:
  - <primary-es>: 主日志 ES，存储大部分业务日志
  - <secondary-es>: 特定业务日志

### 4. 容灾数据中心（DR IDC）
- k8s 集群: <your-dr-cluster>
- Fluent-Bit: 采集本地 k8s 集群日志
- Kafka: <dr-kafka>
  - BCP 容灾用，非主链路
  - 通过 kafka_sync 与主数据中心同步
- Logstash: 从本地 Kafka 消费，写入本地 ES 或同步到主数据中心
- Elasticsearch: 本地 ES（容灾备份）

## 日志流向（主链路）

```
┌─────────────┐     ┌──────────┐     ┌─────────────────┐     ┌───────────┐     ┌──────────────┐
│  k8s Pod    │────▶│ Fluent-  │────▶│     Kafka        │────▶│ Logstash  │────▶│ Elasticsearch│
│  (容器日志)  │     │  Bit     │     │  (消息队列)       │     │ (消费处理) │     │  (存储检索)   │
└─────────────┘     └──────────┘     └─────────────────┘     └───────────┘     └──────────────┘
```

### 云上日志链路

```
阿里云 ACK Pod → Fluent-Bit → aliyun-kafka → [kafka_sync] → 主 Kafka → Logstash → 主 ES
腾讯云 TKE Pod → Fluent-Bit → tencent-ckafka → [kafka_sync] → 主 Kafka → Logstash → 主 ES
```

### 主数据中心本地日志链路

```
主 IDC Pod → Fluent-Bit → 主 Kafka → Logstash → 主 ES
```

### 容灾链路

```
容灾 IDC Pod → Fluent-Bit → 容灾 Kafka → [kafka_sync] → 主 Kafka → Logstash → ES
```

## 组件与 MCP 工具对应关系

| 组件 | MCP Server | 职责 |
|------|-----------|------|
| Fluent-Bit | fluentbit-mcp | 日志采集规则管理、采集状态查询 |
| Kafka | kafka-mcp | Topic/消费组/Broker 管理、消费积压监控 |
| Logstash | logstash-mcp | Pipeline 管理、日志流向追踪 |
| Elasticsearch | elasticsearch-mcp | 索引管理、日志搜索、集群监控 |

## 命名规范
- IDC 机房名称使用英文，如 Datacenter-A、Datacenter-B
- kafka_sync 管道描述为"跨机房 Kafka 同步"
- 云厂商名称：Aliyun/ACK、Tencnt/TKE

## 常见排查场景

### 日志丢失排查
1. fluentbit-mcp: 检查采集规则是否匹配、Fluent-Bit 是否正常运行
2. kafka-mcp: 检查 Topic 是否有数据写入（Offset 增长）、消费组是否有积压
3. logstash-mcp: 检查 Pipeline 是否正常运行、是否有错误日志
4. elasticsearch-mcp: 检查目标索引是否存在、文档数是否增长

### 日志延迟排查
1. kafka-mcp: 检查消费积压（Consumer Lag）
2. logstash-mcp: 检查 Pipeline 处理速率
3. elasticsearch-mcp: 检查写入延迟、Bulk 拒绝

### 跨机房同步问题
1. kafka-mcp: 检查源 Kafka 和目标 Kafka 的 Topic Offset
2. logstash-mcp: 检查 kafka_sync Pipeline 状态
3. fluentbit-mcp: 确认采集规则中的 Kafka Topic 配置
