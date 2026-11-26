package verifylive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"alt/scripts/altharness/internal/fsperm"
)

var forbiddenProdPorts = map[int]struct{}{
	80:    {},
	443:   {},
	4173:  {},
	4433:  {},
	4434:  {},
	5432:  {},
	5434:  {},
	6379:  {},
	6432:  {},
	7700:  {},
	8080:  {},
	8443:  {},
	8888:  {},
	9000:  {},
	9001:  {},
	9110:  {},
	9200:  {},
	9201:  {},
	9202:  {},
	9250:  {},
	9443:  {},
	11434: {},
	11436: {},
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type childSpec struct {
	Args         []string
	Dir          string
	Env          []string
	ReadyFile    string
	StopFile     string
	ReadyTimeout time.Duration
}

type liveChild struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stopFile string
	logs     *syncBuffer
	done     chan error
	baseURL  string
	stopOnce sync.Once
	stopErr  error
}

func startLoopbackChild(ctx context.Context, spec childSpec) (*liveChild, error) {
	if len(spec.Args) == 0 {
		return nil, errors.New("empty child command")
	}
	if spec.ReadyTimeout <= 0 {
		spec.ReadyTimeout = 30 * time.Second
	}
	if spec.StopFile == "" {
		spec.StopFile = spec.ReadyFile + ".stop"
	}

	cmd := exec.CommandContext(ctx, spec.Args[0], spec.Args[1:]...)
	cmd.Dir = spec.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Inherit parent environment without COMPOSE_PROJECT_NAME=alt so children never
	// accidentally target the production Compose project.
	env := make([]string, 0, len(os.Environ())+len(spec.Env)+2)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "COMPOSE_PROJECT_NAME=") || strings.HasPrefix(kv, "ALT_VERIFY_PROJECT=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "ALT_VERIFYLIVE_READY_FILE="+spec.ReadyFile)
	env = append(env, "ALT_VERIFYLIVE_STOP_FILE="+spec.StopFile)
	env = append(env, spec.Env...)
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	logs := &syncBuffer{}
	cmd.Stdout = logs
	cmd.Stderr = logs

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start child %s: %w", spec.Args[0], err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	child := &liveChild{
		cmd:      cmd,
		stdin:    stdin,
		stopFile: spec.StopFile,
		logs:     logs,
		done:     done,
	}

	deadline := time.NewTimer(spec.ReadyTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if raw, rerr := os.ReadFile(spec.ReadyFile); rerr == nil {
			line := strings.TrimSpace(string(raw))
			if line != "" {
				if err := validateLoopbackURL(line); err != nil {
					_ = child.Stop()
					return nil, fmt.Errorf("child bound unsafe address %q: %w", line, err)
				}
				child.baseURL = line
				return child, nil
			}
		}
		select {
		case waitErr := <-done:
			_ = stdin.Close()
			return nil, fmt.Errorf("child exited before ready (err=%w):\n%s", waitErr, logs.String())
		case <-ctx.Done():
			_ = child.Stop()
			return nil, fmt.Errorf("cancelled waiting for child ready: %w\n%s", ctx.Err(), logs.String())
		case <-deadline.C:
			_ = child.Stop()
			return nil, fmt.Errorf("timed out after %s waiting for child ready:\n%s", spec.ReadyTimeout, logs.String())
		case <-ticker.C:
		}
	}
}

func (c *liveChild) Stop() string {
	c.stopOnce.Do(func() {
		if c.stopFile != "" {
			_ = os.WriteFile(c.stopFile, []byte("stop\n"), fsperm.Secret)
		}
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		grace := time.NewTimer(3 * time.Second)
		defer grace.Stop()
		select {
		case c.stopErr = <-c.done:
			return
		case <-grace.C:
		}
		if c.cmd.Process != nil {
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
		}
		c.stopErr = <-c.done
	})
	return c.logs.String()
}

func validateLoopbackURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" {
		return fmt.Errorf("expected http scheme, got %q", u.Scheme)
	}
	if u.Hostname() != "127.0.0.1" {
		return fmt.Errorf("expected 127.0.0.1 loopback host, got %q", u.Hostname())
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port %q", u.Port())
	}
	if _, forbidden := forbiddenProdPorts[port]; forbidden {
		return fmt.Errorf("port %d is reserved by Alt production stack", port)
	}
	return nil
}

func loopbackHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DisableKeepAlives:     true,
			ExpectContinueTimeout: 2 * time.Second,
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type goOverlay struct {
	Replace map[string]string `json:"Replace"`
}

func writeOverlay(tmpDir, targetAbsPath, replacementFileName, replacementSource string) (string, error) {
	srcPath := filepath.Join(tmpDir, replacementFileName)
	if err := os.WriteFile(srcPath, []byte(replacementSource), fsperm.File); err != nil {
		return "", fmt.Errorf("write overlay source: %w", err)
	}
	overlayPath := filepath.Join(tmpDir, "overlay.json")
	payload, err := json.Marshal(goOverlay{
		Replace: map[string]string{
			targetAbsPath: srcPath,
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal overlay: %w", err)
	}
	if err := os.WriteFile(overlayPath, payload, fsperm.File); err != nil {
		return "", fmt.Errorf("write overlay.json: %w", err)
	}
	return overlayPath, nil
}
