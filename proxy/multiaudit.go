package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zorneth/osg-runtime/gatewayclient"
)

// MultiAudit writes OCSF lines to several sinks (stderr, daily file, gateway).
type MultiAudit struct {
	mu       sync.Mutex
	writers  []io.Writer
	file     *os.File
	fileDay  string
	logDir   string
	gw       *gatewayclient.Client
	sandbox  string
	source   string
	buf      []gatewayclient.LogLine
	lastPush time.Time
}

// NewMultiAudit builds sinks. logDir empty skips file; gatewayURL/sandbox empty skips push.
func NewMultiAudit(primary io.Writer, logDir, gatewayURL, sandbox string) *MultiAudit {
	if primary == nil {
		primary = os.Stderr
	}
	m := &MultiAudit{
		writers: []io.Writer{primary},
		logDir:  strings.TrimSpace(logDir),
		sandbox: strings.TrimSpace(sandbox),
		source:  "proxy",
	}
	if gatewayURL != "" && m.sandbox != "" {
		m.gw = gatewayclient.New(gatewayURL)
	}
	_ = m.rotateFileLocked()
	return m
}

func (m *MultiAudit) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.rotateFileLocked()
	n := len(p)
	for _, w := range m.writers {
		_, _ = w.Write(p)
	}
	if m.file != nil {
		_, _ = m.file.Write(p)
	}
	text := strings.TrimRight(string(p), "\r\n")
	if text != "" && m.gw != nil {
		level := "INFO"
		if strings.Contains(text, "[MED]") {
			level = "MED"
		} else if strings.Contains(text, "[HIGH]") {
			level = "HIGH"
		} else if strings.Contains(text, " OCSF ") {
			level = "OCSF"
		}
		m.buf = append(m.buf, gatewayclient.LogLine{
			TS: time.Now().UTC(), Source: m.source, Level: level, Text: text,
		})
		if len(m.buf) >= 8 || time.Since(m.lastPush) > 2*time.Second {
			m.flushGWLocked()
		}
	}
	return n, nil
}

func (m *MultiAudit) flushGWLocked() {
	if m.gw == nil || len(m.buf) == 0 {
		return
	}
	lines := m.buf
	m.buf = nil
	m.lastPush = time.Now()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = m.gw.PostLogs(ctx, m.sandbox, lines)
	}()
}

func (m *MultiAudit) rotateFileLocked() error {
	if m.logDir == "" {
		return nil
	}
	day := time.Now().UTC().Format("2006-01-02")
	if m.file != nil && m.fileDay == day {
		return nil
	}
	if m.file != nil {
		_ = m.file.Close()
		m.file = nil
	}
	if err := os.MkdirAll(m.logDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(m.logDir, "osg."+day+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	m.file = f
	m.fileDay = day
	// prune old files (keep 3)
	ents, _ := os.ReadDir(m.logDir)
	var logs []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "osg.") && strings.HasSuffix(e.Name(), ".log") {
			logs = append(logs, e.Name())
		}
	}
	if len(logs) > 3 {
		// best-effort delete oldest by name (YYYY-MM-DD sorts)
		for _, name := range logs[:len(logs)-3] {
			_ = os.Remove(filepath.Join(m.logDir, name))
		}
	}
	return nil
}

// Close flushes gateway buffer and file.
func (m *MultiAudit) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushGWLocked()
	if m.file != nil {
		return m.file.Close()
	}
	return nil
}

// Ensure MultiAudit is an io.Writer.
var _ io.Writer = (*MultiAudit)(nil)

// Discard scanner helper for tests.
func scanLines(r io.Reader, fn func(string)) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fn(sc.Text())
	}
}

// LifecycleReady emits a LIFECYCLE line onto w.
func LifecycleReady(w io.Writer, detail string) {
	if w == nil {
		return
	}
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = fmt.Fprintf(w, "%s OCSF LIFECYCLE:START [INFO] ALLOWED %s\n", ts, detail)
}
