// Package tools 提供共享的辅助函数，供 tools.go 和 monitoring.go 使用。
package tools

import (
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ===== 数据真实性提醒 =====

const dataIntegrityReminder = "\n\n[IMPORTANT] 以上数据均来自实际工具查询结果。请严格基于上述返回数据进行分析和回答，严禁编造、捏造或虚构任何未在上述结果中出现的数据。如果数据不足以回答用户问题，请如实告知。"

// ===== 错误码常量 =====

const (
	ErrCodeParam      = "PARAM_ERROR"
	ErrCodeDatasource = "DATASOURCE_ERROR"
	ErrCodeES         = "ES_ERROR"
	ErrCodeNotFound   = "NOT_FOUND"
)

// ===== MCP 结果构造 =====

// createErrorResult 创建标准化错误结果，包含错误码前缀
func createErrorResult(message string) mcp.CallToolResult {
	return mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("错误: %s\n\n[IMPORTANT] 工具调用失败，请如实告知用户查询失败，严禁编造数据。", message)}},
		IsError: true,
	}
}

// createParamError 创建参数校验错误
func createParamError(param string, reason string) mcp.CallToolResult {
	msg := fmt.Sprintf("[%s] 参数 '%s' %s", ErrCodeParam, param, reason)
	return mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("错误: %s\n\n[IMPORTANT] 工具调用失败，请如实告知用户查询失败，严禁编造数据。", msg)}},
		IsError: true,
	}
}

// createDatasourceError 创建数据源错误
func createDatasourceError(datasource string, detail string, available []string) mcp.CallToolResult {
	msg := fmt.Sprintf("[%s] 数据源 '%s' %s。可用数据源: %v", ErrCodeDatasource, datasource, detail, available)
	return mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("错误: %s\n\n[IMPORTANT] 工具调用失败，请如实告知用户查询失败，严禁编造数据。", msg)}},
		IsError: true,
	}
}

// createESError 创建 Elasticsearch 操作错误
func createESError(operation string, detail string) mcp.CallToolResult {
	msg := fmt.Sprintf("[%s] %s失败: %s", ErrCodeES, operation, detail)
	return mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("错误: %s\n\n[IMPORTANT] 工具调用失败，请如实告知用户查询失败，严禁编造数据。", msg)}},
		IsError: true,
	}
}

// createNotFoundError 创建资源未找到错误
func createNotFoundError(resourceType string, identifier string) mcp.CallToolResult {
	msg := fmt.Sprintf("[%s] %s '%s' 不存在", ErrCodeNotFound, resourceType, identifier)
	return mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("错误: %s\n\n[IMPORTANT] 工具调用失败，请如实告知用户查询失败，严禁编造数据。", msg)}},
		IsError: true,
	}
}

// createSuccessResult 创建带有结构化数据的标准化成功结果
func createSuccessResult(text string, data interface{}) mcp.CallToolResult {
	content := []mcp.Content{&mcp.TextContent{Text: text}}
	result := mcp.CallToolResult{Content: content, IsError: false}

	if data != nil {
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err == nil {
			content = append(content, &mcp.TextContent{Text: fmt.Sprintf("数据详情:\n%s", string(jsonData))})
			result.Content = content

			var normalized interface{}
			if err := json.Unmarshal(jsonData, &normalized); err == nil {
				switch v := normalized.(type) {
				case []interface{}:
					result.StructuredContent = map[string]interface{}{"data": v, "count": len(v)}
				case map[string]interface{}:
					result.StructuredContent = v
				default:
					result.StructuredContent = map[string]interface{}{"data": normalized}
				}
			}
		}
	}
	result.Content = append(result.Content, &mcp.TextContent{Text: dataIntegrityReminder})
	return result
}

// createSimpleSuccessResult 创建只有文本的简单成功结果
func createSimpleSuccessResult(text string) mcp.CallToolResult {
	return mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text + dataIntegrityReminder}},
		IsError: false,
	}
}

// ===== 参数提取 =====

// getArg 从参数 map 中提取字符串值
func getArg(args map[string]interface{}, key string) string {
	v, _ := args[key].(string)
	return v
}

// nodeFilter 返回节点名称过滤正则，空值返回 ".*"
func nodeFilter(name string) string {
	if name == "" {
		return ".*"
	}
	return name
}
