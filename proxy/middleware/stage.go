// Package middleware runs optional HTTP/WS stages before credential rewrite.
package middleware

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

// Request is the inspectable egress request passed to stages.
type Request struct {
	Host    string            `json:"host"`
	Port    int               `json:"port"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Decision is allow/deny (+ optional header mutations).
type Decision struct {
	Allow         bool              `json:"allow"`
	Reason        string            `json:"reason,omitempty"`
	MutateHeaders map[string]string `json:"mutate_headers,omitempty"`
}

// Stage evaluates one middleware hop.
type Stage interface {
	Name() string
	Evaluate(ctx context.Context, req Request) (Decision, error)
}

// Pipeline runs stages in order; first deny wins. Empty pipeline = allow.
type Pipeline struct {
	Stages []Stage
}

// Run evaluates all stages.
func (p *Pipeline) Run(ctx context.Context, req Request) (Decision, error) {
	if p == nil || len(p.Stages) == 0 {
		return Decision{Allow: true, Reason: "no middleware"}, nil
	}
	acc := map[string]string{}
	for _, s := range p.Stages {
		dec, err := s.Evaluate(ctx, req)
		if err != nil {
			return Decision{}, fmt.Errorf("middleware %s: %w", s.Name(), err)
		}
		if !dec.Allow {
			if dec.Reason == "" {
				dec.Reason = "denied by " + s.Name()
			}
			return dec, nil
		}
		for k, v := range dec.MutateHeaders {
			if req.Headers == nil {
				req.Headers = map[string]string{}
			}
			req.Headers[k] = v
			acc[k] = v
		}
	}
	return Decision{Allow: true, Reason: "middleware allow", MutateHeaders: acc}, nil
}

// JWTStub verifies that Authorization Bearer is present and optionally matches aud claim (unverified parse).
type JWTStub struct {
	Audience string
	Required bool
}

func (j *JWTStub) Name() string { return "jwt_stub" }

func (j *JWTStub) Evaluate(_ context.Context, req Request) (Decision, error) {
	auth := ""
	if req.Headers != nil {
		auth = req.Headers["Authorization"]
		if auth == "" {
			auth = req.Headers["authorization"]
		}
	}
	if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		if j.Required {
			return Decision{Allow: false, Reason: "jwt_stub: missing bearer token"}, nil
		}
		return Decision{Allow: true, Reason: "jwt_stub: optional token absent"}, nil
	}
	token := strings.TrimSpace(auth[7:])
	if j.Audience == "" {
		if token == "" {
			return Decision{Allow: false, Reason: "jwt_stub: empty token"}, nil
		}
		return Decision{Allow: true, Reason: "jwt_stub: bearer present"}, nil
	}
	// Unverified JWT payload parse (MVP stub — not cryptographic verification).
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return Decision{Allow: false, Reason: "jwt_stub: malformed jwt"}, nil
	}
	payload, err := decodeSegment(parts[1])
	if err != nil {
		return Decision{Allow: false, Reason: "jwt_stub: bad payload"}, nil
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Decision{Allow: false, Reason: "jwt_stub: payload json"}, nil
	}
	aud, _ := claims["aud"].(string)
	if aud == "" {
		if arr, ok := claims["aud"].([]any); ok && len(arr) > 0 {
			aud, _ = arr[0].(string)
		}
	}
	if aud != j.Audience {
		return Decision{Allow: false, Reason: "jwt_stub: audience mismatch"}, nil
	}
	return Decision{Allow: true, Reason: "jwt_stub: aud ok"}, nil
}

// RemoteStage POSTs the request JSON to URL and expects a Decision JSON body.
type RemoteStage struct {
	URL        string
	FailClosed bool
	HTTP       *http.Client
	Label      string
}

func (r *RemoteStage) Name() string {
	if r.Label != "" {
		return r.Label
	}
	return "remote"
}

func (r *RemoteStage) Evaluate(ctx context.Context, req Request) (Decision, error) {
	cli := r.HTTP
	if cli == nil {
		cli = &http.Client{Timeout: 5 * time.Second}
	}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(body))
	if err != nil {
		return Decision{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	res, err := cli.Do(httpReq)
	if err != nil {
		if r.FailClosed {
			return Decision{Allow: false, Reason: "remote stage unreachable"}, nil
		}
		return Decision{Allow: true, Reason: "remote stage fail-open"}, nil
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		if r.FailClosed {
			return Decision{Allow: false, Reason: fmt.Sprintf("remote stage http %s", res.Status)}, nil
		}
		return Decision{Allow: true, Reason: "remote stage fail-open http"}, nil
	}
	var dec Decision
	if err := json.Unmarshal(b, &dec); err != nil {
		if r.FailClosed {
			return Decision{Allow: false, Reason: "remote stage bad json"}, nil
		}
		return Decision{Allow: true, Reason: "remote stage fail-open json"}, nil
	}
	return dec, nil
}
