package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/zorneth/osg-core/policy"
)

// parseMCPRequest extracts JSON-RPC method and optional tools/call name from an HTTP body.
// Returns method, tool, restored body reader, error. Fail-closed on invalid JSON for MCP.
func parseMCPRequest(req *http.Request) (method, tool string, err error) {
	if req.Body == nil {
		return "", "", fmt.Errorf("mcp: empty body")
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 256<<10))
	_ = req.Body.Close()
	if err != nil {
		return "", "", err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))

	var msg struct {
		Method string `json:"method"`
		Params *struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		return "", "", fmt.Errorf("mcp: invalid json-rpc: %w", err)
	}
	method = strings.TrimSpace(msg.Method)
	if method == "" {
		return "", "", fmt.Errorf("mcp: method required")
	}
	if method == "tools/call" && msg.Params != nil {
		tool = strings.TrimSpace(msg.Params.Name)
	}
	return method, tool, nil
}

// mcpDecidePath builds MatchHTTP path: URL path + NUL + tool (tool may be empty).
func mcpDecidePath(urlPath, tool string) string {
	if urlPath == "" {
		urlPath = "/"
	}
	if tool == "" {
		return urlPath
	}
	return urlPath + "\x00" + tool
}

func isMCPRule(rule *policy.AllowRule) bool {
	return rule != nil && strings.EqualFold(strings.TrimSpace(rule.Protocol), policy.ProtocolMCP)
}
