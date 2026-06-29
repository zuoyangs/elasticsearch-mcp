// Package prompts 提供 Elasticsearch MCP 服务器的 Prompt 模板。
package prompts

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册所有 Prompt 到 MCP 服务器
func Register(s *mcp.Server) {
	s.AddPrompt(promptClusterInspection, handleClusterInspection)
	s.AddPrompt(promptHealthDiagnosis, handleHealthDiagnosis)
	s.AddPrompt(promptPerformanceTroubleshoot, handlePerformanceTroubleshoot)
	s.AddPrompt(promptDiskCapacityAnalysis, handleDiskCapacityAnalysis)
	s.AddPrompt(promptJVMGCDiagnosis, handleJVMGCDiagnosis)
	s.AddPrompt(promptNetworkDiagnosis, handleNetworkDiagnosis)
	s.AddPrompt(promptIndexLifecycleCheck, handleIndexLifecycleCheck)
	s.AddPrompt(promptSlowQueryAnalysis, handleSlowQueryAnalysis)
	s.AddPrompt(promptLogSearch, handleLogSearch)
	s.AddPrompt(promptIndexCleanupAnalysis, handleIndexCleanupAnalysis)
	s.AddPrompt(promptLogMissingFromES, handleLogMissingFromES)
	s.AddPrompt(promptESCostAnalysis, handleESCostAnalysis)
	s.AddPrompt(promptESCapacityPlanning, handleESCapacityPlanning)
	s.AddPrompt(promptESPerformanceDeepDive, handleESPerformanceDeepDive)
	s.AddPrompt(promptESShardCapacityManagement, handleESShardCapacityManagement)
	s.AddPrompt(promptESWritePerformanceAnalysis, handleESWritePerformanceAnalysis)
	s.AddPrompt(promptESSearchPerformanceAnalysis, handleESSearchPerformanceAnalysis)
	s.AddPrompt(promptLogInvestigation, handleLogInvestigation)
}
