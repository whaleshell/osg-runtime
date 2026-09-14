package gatewayclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ExecResult is the relay exec response.
type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

// GetSandbox GET /v1/sandboxes/{name}.
func (c *Client) GetSandbox(ctx context.Context, name string) (Sandbox, error) {
	var sb Sandbox
	if err := c.get(ctx, "/v1/sandboxes/"+name, &sb); err != nil {
		return Sandbox{}, err
	}
	return sb, nil
}

// Exec posts argv to the gateway relay (requires osg-agent polling in the sandbox).
func (c *Client) Exec(ctx context.Context, name string, argv []string) (ExecResult, error) {
	body, err := json.Marshal(map[string]any{"argv": argv})
	if err != nil {
		return ExecResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/relay/"+name+"/exec", bytes.NewReader(body))
	if err != nil {
		return ExecResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return ExecResult{}, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return ExecResult{}, fmt.Errorf("gateway relay exec: %s: %s", res.Status, bytes.TrimSpace(b))
	}
	var out ExecResult
	if err := json.Unmarshal(b, &out); err != nil {
		return ExecResult{}, err
	}
	return out, nil
}
