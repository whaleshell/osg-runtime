// Command osg-agent dials the gateway relay WebSocket and runs exec requests.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	gateway := os.Getenv("OSG_GATEWAY")
	name := os.Getenv("OSG_SANDBOX")
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--gateway":
			i++
			gateway = args[i]
		case "--name":
			i++
			name = args[i]
		case "-h", "--help":
			fmt.Fprintf(os.Stderr, "usage: osg-agent --gateway URL --name SANDBOX\n")
			return nil
		default:
			return fmt.Errorf("unknown flag %q", args[i])
		}
	}
	if gateway == "" || name == "" {
		return fmt.Errorf("osg-agent: --gateway and --name required")
	}
	base := strings.TrimRight(gateway, "/")
	fmt.Fprintf(os.Stderr, "osg-agent: registering %s at %s\n", name, base)

	// Long-poll style relay: poll for jobs, post results (works without gorilla/websocket).
	client := &http.Client{Timeout: 65 * time.Second}
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		job, err := pollJob(ctx, client, base, name)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "osg-agent: poll: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if job == nil {
			continue
		}
		out, code := runJob(job.Argv)
		_ = postResult(client, base, name, job.ID, out, code)
	}
}

type job struct {
	ID   string   `json:"id"`
	Argv []string `json:"argv"`
}

func pollJob(ctx context.Context, client *http.Client, base, name string) (*job, error) {
	u := base + "/v1/relay/" + url.PathEscape(name) + "/poll"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", res.Status)
	}
	var j job
	if err := json.NewDecoder(res.Body).Decode(&j); err != nil {
		return nil, err
	}
	return &j, nil
}

func runJob(argv []string) (string, int) {
	if len(argv) == 0 {
		return "empty argv", 1
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
			buf.WriteString(err.Error())
		}
	}
	return buf.String(), code
}

func postResult(client *http.Client, base, name, id, out string, code int) error {
	body, _ := json.Marshal(map[string]any{"id": id, "exit_code": code, "output": out})
	u := base + "/v1/relay/" + url.PathEscape(name) + "/result"
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}
