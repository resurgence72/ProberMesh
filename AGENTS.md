# ProberMesh 项目规则

## gopls MCP 使用规则

本项目已接入 gopls MCP（通过 `gopls mcp` 启动）。当需要理解或修改 Go 代码时，优先使用 gopls 的 MCP 工具：

- **开始任何 Go 会话时**：先调用 `go_workspace` 了解 Go workspace 结构；若在 Go workspace 内，紧接着运行 `go_vulncheck` 检查已知安全漏洞。
- **阅读理解 Go 代码时**：用 `go_search` 做模糊符号搜索，用 `go_file_context` 理解文件与同包文件的依赖，用 `go_package_api` 查看包的公开 API。
- **修改 Go 代码前**：对要改的符号先用 `go_symbol_references` 找到所有引用，评估影响面。
- **每次修改后**：调用 `go_diagnostics` 检查编译/分析错误，修复后再继续；涉及 go.mod 依赖变更时运行 `go_vulncheck`。
- **测试**：仅在 `go_diagnostics` 无错误后，对改动过的包运行 `go test`。

## CodeGraph 使用规则

在 .codegraph/ 索引存在时，优先用 codegraph 的 `codegraph_explore` 工具（或 `codegraph explore` 命令）回答"X 如何工作""X 如何到达 Y"、架构类问题，而不是直接用 grep/find。
