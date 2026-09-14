// Package gatewayclient talks to osg-gateway HTTP API.
package gatewayclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a tiny HTTP client for osg-gateway.
type Client struct {
	Base   string
	HTTP   *http.Client
}

// New returns a client for base URL (for example http://127.0.0.1:7443).
func New(base string) *Client {
	return &Client{
		Base: strings.TrimRight(base, "/"),
		HTTP: &http.Client{Timeout: 10 * time.Second},
	}
}

// Healthz hits /healthz.
func (c *Client) Healthz(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/healthz", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Info hits /v1/info.
func (c *Client) Info(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/v1/info", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Sandbox is the registry payload.
type Sandbox struct {
	Name              string            `json:"name"`
	ID                string            `json:"id,omitempty"`
	Image             string            `json:"image,omitempty"`
	Network           string            `json:"network,omitempty"`
	Status            string            `json:"status,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	BasePolicyYAML    string            `json:"base_policy_yaml,omitempty"`
	AttachedProviders []string          `json:"attached_providers,omitempty"`
}

// UpsertSandbox PUT /v1/sandboxes/{name}.
func (c *Client) UpsertSandbox(ctx context.Context, sb Sandbox) error {
	b, err := json.Marshal(sb)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/sandboxes/"+sb.Name, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("gateway upsert: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// DeleteSandbox DELETE /v1/sandboxes/{name}.
func (c *Client) DeleteSandbox(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.Base+"/v1/sandboxes/"+name, nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("gateway delete: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// ListSandboxes GET /v1/sandboxes.
func (c *Client) ListSandboxes(ctx context.Context) ([]Sandbox, error) {
	var out struct {
		Sandboxes []Sandbox `json:"sandboxes"`
	}
	if err := c.get(ctx, "/v1/sandboxes", &out); err != nil {
		return nil, err
	}
	return out.Sandboxes, nil
}

// GetGlobalPolicy GET /v1/policy/global (YAML bytes).
func (c *Client) GetGlobalPolicy(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/v1/policy/global", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("gateway global policy: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return body, nil
}

// PutGlobalPolicy PUT /v1/policy/global with YAML body.
func (c *Client) PutGlobalPolicy(ctx context.Context, yaml []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/policy/global", bytes.NewReader(yaml))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/yaml")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("gateway put global policy: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// ProfileInfo is a catalog entry summary.
type ProfileInfo struct {
	ID     string `json:"id"`
	Source string `json:"source"`
}

// ListProfiles GET /v1/profiles.
func (c *Client) ListProfiles(ctx context.Context) ([]ProfileInfo, error) {
	var out struct {
		Profiles []ProfileInfo `json:"profiles"`
	}
	if err := c.get(ctx, "/v1/profiles", &out); err != nil {
		return nil, err
	}
	return out.Profiles, nil
}

// PutProfile PUT /v1/profiles/{id} with YAML body.
func (c *Client) PutProfile(ctx context.Context, id string, yaml []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/profiles/"+id, bytes.NewReader(yaml))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/yaml")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("gateway put profile: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// ProviderRecord is a gateway provider instance (env refs only).
type ProviderRecord struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	EnvVars []string `json:"env_vars,omitempty"`
}

// PutProvider PUT /v1/providers/{name}.
func (c *Client) PutProvider(ctx context.Context, rec ProviderRecord) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/providers/"+rec.Name, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("gateway put provider: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// ListProviders GET /v1/providers.
func (c *Client) ListProviders(ctx context.Context) ([]ProviderRecord, error) {
	var out struct {
		Providers []ProviderRecord `json:"providers"`
	}
	if err := c.get(ctx, "/v1/providers", &out); err != nil {
		return nil, err
	}
	return out.Providers, nil
}

// AttachProvider PUT /v1/sandboxes/{sandbox}/providers/{provider}.
func (c *Client) AttachProvider(ctx context.Context, sandbox, provider string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Base+"/v1/sandboxes/"+sandbox+"/providers/"+provider, nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("gateway attach provider: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// DetachProvider DELETE /v1/sandboxes/{sandbox}/providers/{provider}.
func (c *Client) DetachProvider(ctx context.Context, sandbox, provider string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.Base+"/v1/sandboxes/"+sandbox+"/providers/"+provider, nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("gateway detach provider: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return nil
}

// EffectivePolicy GET /v1/sandboxes/{name}/effective-policy (YAML).
func (c *Client) EffectivePolicy(ctx context.Context, sandbox string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/v1/sandboxes/"+sandbox+"/effective-policy", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("gateway effective-policy: %s: %s", res.Status, bytes.TrimSpace(body))
	}
	return body, nil
}

func (c *Client) get(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("gateway %s: %s: %s", path, res.Status, bytes.TrimSpace(body))
	}
	return json.NewDecoder(res.Body).Decode(dest)
}
