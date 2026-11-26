package verifylive

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"alt/scripts/altharness/internal/fsperm"
)

func executeProbe(ctx context.Context, repoRoot, name string, checkC07Mutation bool) (ProbeResult, string) {
	switch name {
	case "sse":
		return runSSEProbe(ctx, repoRoot, checkC07Mutation)
	case "body-limit":
		return runBodyLimitProbe(ctx, repoRoot)
	case "auth-hub":
		return runAuthHubProbe(ctx, repoRoot)
	case "inference-proxy":
		return runInferenceProxyProbe(ctx, repoRoot)
	default:
		return ProbeResult{
			Name:  name,
			OK:    false,
			Error: fmt.Sprintf("unknown probe %q", name),
		}, ""
	}
}

func addCheck(res *ProbeResult, name string, ok bool, detail string) {
	res.Checks = append(res.Checks, ProbeCheck{Name: name, OK: ok, Detail: detail})
	if !ok {
		res.OK = false
	}
}

// ============================================================================
// Probe 1: SSE (C-07) — real news-creator FastAPI/Uvicorn on 127.0.0.1:0
// ============================================================================

const sseLiveServerPy = `"""Isolated loopback FastAPI/Uvicorn server driving news_creator.handler.sse_stream."""

import asyncio
import os
import socket
import sys
from collections.abc import AsyncGenerator
from pathlib import Path

import uvicorn
from fastapi import FastAPI, Request

sys.path.insert(0, os.environ["ALT_NEWS_CREATOR_APP"])
from news_creator.handler.sse_stream import SSEHeartbeatConfig, create_sse_response

app = FastAPI()


async def _normal_stream() -> AsyncGenerator[str]:
    yield "alpha"
    await asyncio.sleep(0.01)
    yield "beta"


class _FailingCloseAsyncGen:
    """AsyncGenerator-compatible stream whose aclose() raises RuntimeError."""

    def __init__(self) -> None:
        self._emitted = False

    def __aiter__(self) -> "_FailingCloseAsyncGen":
        return self

    async def __anext__(self) -> str:
        if not self._emitted:
            self._emitted = True
            return "partial-chunk"
        raise StopAsyncIteration

    async def aclose(self) -> None:
        raise RuntimeError("simulated upstream aclose failure")


@app.get("/health")
async def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/probe/sse-normal")
async def probe_sse_normal(request: Request):
    return create_sse_response(
        _normal_stream(),
        request,
        stream_id="verify-live-normal",
        error_message="summary generation failed",
        heartbeat_config=SSEHeartbeatConfig(interval_seconds=0.5, idle_threshold_seconds=0.5),
    )


@app.post("/probe/sse-aclose-error")
async def probe_sse_aclose_error(request: Request):
    return create_sse_response(
        _FailingCloseAsyncGen(),  # type: ignore[arg-type]
        request,
        stream_id="verify-live-c07",
        error_message="summary generation failed",
        heartbeat_config=SSEHeartbeatConfig(interval_seconds=0.5, idle_threshold_seconds=0.5),
    )


async def _watch_stdin(server: uvicorn.Server) -> None:
    loop = asyncio.get_running_loop()
    await loop.run_in_executor(None, sys.stdin.read)
    server.should_exit = True


async def _main() -> None:
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.bind(("127.0.0.1", 0))
    sock.listen(16)
    port = sock.getsockname()[1]

    config = uvicorn.Config(app, host="127.0.0.1", port=port, log_level="warning")
    server = uvicorn.Server(config)
    watcher = asyncio.create_task(_watch_stdin(server))

    ready_file = Path(os.environ["ALT_VERIFYLIVE_READY_FILE"])
    tmp_ready = ready_file.with_suffix(".tmp")

    async def _signal_ready() -> None:
        while not server.started:
            await asyncio.sleep(0.01)
        tmp_ready.write_text(f"http://127.0.0.1:{port}\n", encoding="utf-8")
        tmp_ready.replace(ready_file)

    ready_task = asyncio.create_task(_signal_ready())
    try:
        await server.serve(sockets=[sock])
    finally:
        ready_task.cancel()
        watcher.cancel()
        sock.close()


if __name__ == "__main__":
    asyncio.run(_main())
`

func runSSEProbe(ctx context.Context, repoRoot string, checkC07Mutation bool) (res ProbeResult, logOut string) {
	start := time.Now()
	res = ProbeResult{
		Name:    "sse",
		Service: "news-creator/app",
		OK:      true,
	}
	var logBuf bytes.Buffer
	defer func() {
		res.DurationMS = time.Since(start).Milliseconds()
	}()

	pyCmd, pyLabel, err := resolvePythonForSSE(ctx, repoRoot)
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, err.Error() + "\n"
	}

	tmpDir, err := os.MkdirTemp("", "alt-verifylive-sse-*")
	if err != nil {
		res.OK = false
		res.Error = fmt.Sprintf("mkdirtemp: %v", err)
		return res, res.Error + "\n"
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	scriptPath := filepath.Join(tmpDir, "sse_live_server.py")
	if err := os.WriteFile(scriptPath, []byte(sseLiveServerPy), fsperm.File); err != nil {
		res.OK = false
		res.Error = fmt.Sprintf("write sse server script: %v", err)
		return res, res.Error + "\n"
	}
	readyFile := filepath.Join(tmpDir, "ready.url")
	args := append(append([]string(nil), pyCmd...), scriptPath)
	child, err := startLoopbackChild(ctx, childSpec{
		Args: args,
		Dir:  filepath.Join(repoRoot, "news-creator", "app"),
		Env: []string{
			"ALT_NEWS_CREATOR_APP=" + filepath.Join(repoRoot, "news-creator", "app"),
			"PYTHONUNBUFFERED=1",
		},
		ReadyFile:    readyFile,
		ReadyTimeout: 25 * time.Second,
	})
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, fmt.Sprintf("python=%s\n%s\n", pyLabel, err.Error())
	}
	res.BindAddr = child.baseURL
	fmt.Fprintf(&logBuf, "python=%s bind=%s\n", pyLabel, child.baseURL)

	client := loopbackHTTPClient(5 * time.Second)

	// 1. /health
	healthReq, err := http.NewRequestWithContext(ctx, http.MethodGet, child.baseURL+"/health", nil)
	if err != nil {
		addCheck(&res, "health", false, err.Error())
	} else if resp, rerr := client.Do(healthReq); rerr != nil {
		addCheck(&res, "health", false, rerr.Error())
	} else {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		ok := resp.StatusCode == http.StatusOK && strings.Contains(string(body), `"ok"`)
		addCheck(&res, "health", ok, fmt.Sprintf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body))))
		fmt.Fprintf(&logBuf, "GET /health -> %d %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// 2. Normal SSE stream
	normReq, err := http.NewRequestWithContext(ctx, http.MethodPost, child.baseURL+"/probe/sse-normal", nil)
	if err != nil {
		addCheck(&res, "sse-normal", false, err.Error())
	} else if resp, rerr := client.Do(normReq); rerr != nil {
		addCheck(&res, "sse-normal", false, rerr.Error())
	} else {
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		ct := resp.Header.Get("Content-Type")
		cc := resp.Header.Get("Cache-Control")
		xab := resp.Header.Get("X-Accel-Buffering")
		bodyStr := string(body)
		ok := readErr == nil &&
			resp.StatusCode == http.StatusOK &&
			strings.Contains(ct, "text/event-stream") &&
			cc == "no-cache" &&
			xab == "no" &&
			strings.Contains(bodyStr, `data: "alpha"`) &&
			strings.Contains(bodyStr, `data: "beta"`)
		addCheck(&res, "sse-normal", ok, fmt.Sprintf("status=%d ct=%q cache=%q x-accel=%q readErr=%v", resp.StatusCode, ct, cc, xab, readErr))
		fmt.Fprintf(&logBuf, "POST /probe/sse-normal -> %d ct=%q body:\n%s\n", resp.StatusCode, ct, bodyStr)
	}

	// 3. C-07 aclose() raising RuntimeError must still emit error event and clean EOF without hanging.
	errReq, err := http.NewRequestWithContext(ctx, http.MethodPost, child.baseURL+"/probe/sse-aclose-error", nil)
	if err != nil {
		addCheck(&res, "sse-aclose-error-terminates", false, err.Error())
	} else if resp, rerr := client.Do(errReq); rerr != nil {
		addCheck(&res, "sse-aclose-error-terminates", false, fmt.Sprintf("request failed or hung: %v", rerr))
	} else {
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		bodyStr := string(body)
		ok := readErr == nil &&
			resp.StatusCode == http.StatusOK &&
			strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") &&
			strings.Contains(bodyStr, `data: "partial-chunk"`) &&
			strings.Contains(bodyStr, "event: error") &&
			strings.Contains(bodyStr, "summary generation failed")
		addCheck(&res, "sse-aclose-error-terminates", ok, fmt.Sprintf("status=%d readErr=%v received_error_event=%v clean_eof=%v",
			resp.StatusCode, readErr, strings.Contains(bodyStr, "event: error"), readErr == nil))
		fmt.Fprintf(&logBuf, "POST /probe/sse-aclose-error -> %d readErr=%v body:\n%s\n", resp.StatusCode, readErr, bodyStr)
	}

	childLogs := child.Stop()
	fmt.Fprintf(&logBuf, "--- child stdout/stderr ---\n%s\n", childLogs)

	if checkC07Mutation {
		beforePath := filepath.Join(repoRoot, "scripts", "harness", "corpus", "C-07", "before.py")
		afterPath := filepath.Join(repoRoot, "scripts", "harness", "corpus", "C-07", "after.py")
		c07Res, c07Err := CheckC07Corpus(ctx, beforePath, afterPath, defaultC07Timeout)
		if c07Err != nil {
			addCheck(&res, "c07-mutation-detection", false, c07Err.Error())
		} else {
			res.C07Mutation = &c07Res
			addCheck(&res, "c07-mutation-detection", c07Res.Detected,
				fmt.Sprintf("before_passed=%v after_passed=%v signal=%s", c07Res.BeforePassed, c07Res.AfterPassed, c07Res.Signal))
			fmt.Fprintf(&logBuf, "C-07 corpus check: detected=%v before=%v(%s) after=%v(%s) signal=%s\n",
				c07Res.Detected, c07Res.BeforePassed, c07Res.BeforeDetail, c07Res.AfterPassed, c07Res.AfterDetail, c07Res.Signal)
		}
	}

	return res, logBuf.String()
}

// ============================================================================
// C-07 Corpus Live SSE Evaluation (stdlib Python asyncio HTTP/1.1 SSE server)
// ============================================================================

const c07CorpusServerPy = `"""Minimal stdlib HTTP/1.1 chunked SSE server evaluating a C-07 forward_stream_chunks module."""

import asyncio
import importlib.util
import json
import os
import sys
from pathlib import Path


def _load_forwarder(module_path: str):
    spec = importlib.util.spec_from_file_location("c07_subject", module_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load module from {module_path}")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod.forward_stream_chunks


class _FailingCloseStream:
    def __init__(self) -> None:
        self._emitted = False

    def __aiter__(self) -> "_FailingCloseStream":
        return self

    async def __anext__(self) -> str:
        if not self._emitted:
            self._emitted = True
            return "chunk-1"
        raise StopAsyncIteration

    async def aclose(self) -> None:
        raise RuntimeError("simulated upstream aclose failure")


async def _write_chunk(writer: asyncio.StreamWriter, text: str) -> None:
    payload = text.encode("utf-8")
    writer.write(f"{len(payload):x}\r\n".encode("ascii") + payload + b"\r\n")
    await writer.drain()


async def _main() -> None:
    forward_stream_chunks = _load_forwarder(os.environ["ALT_C07_MODULE"])
    stop_event = asyncio.Event()

    async def handle_client(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        try:
            await reader.readuntil(b"\r\n\r\n")
            headers = (
                "HTTP/1.1 200 OK\r\n"
                "Content-Type: text/event-stream; charset=utf-8\r\n"
                "Cache-Control: no-cache\r\n"
                "Transfer-Encoding: chunked\r\n"
                "Connection: close\r\n\r\n"
            )
            writer.write(headers.encode("ascii"))
            await writer.drain()

            queue: asyncio.Queue[tuple[str, str] | None] = asyncio.Queue()
            producer = asyncio.create_task(forward_stream_chunks(_FailingCloseStream(), queue))
            try:
                while True:
                    item = await queue.get()
                    if item is None:
                        break
                    kind, val = item
                    if kind == "data":
                        await _write_chunk(writer, f"data: {json.dumps(val)}\n\n")
                    elif kind == "error":
                        await _write_chunk(writer, f"event: error\ndata: {json.dumps({'error': val})}\n\n")
                writer.write(b"0\r\n\r\n")
                await writer.drain()
            finally:
                producer.cancel()
                try:
                    await producer
                except BaseException:
                    pass
        except BaseException:
            pass
        finally:
            writer.close()
            try:
                await writer.wait_closed()
            except BaseException:
                pass

    server = await asyncio.start_server(handle_client, "127.0.0.1", 0)
    port = server.sockets[0].getsockname()[1]
    ready_file = Path(os.environ["ALT_VERIFYLIVE_READY_FILE"])
    tmp_ready = ready_file.with_suffix(".tmp")
    tmp_ready.write_text(f"http://127.0.0.1:{port}\n", encoding="utf-8")
    tmp_ready.replace(ready_file)

    loop = asyncio.get_running_loop()
    await loop.run_in_executor(None, sys.stdin.read)
    stop_event.set()
    server.close()
    await server.wait_closed()


if __name__ == "__main__":
    asyncio.run(_main())
`

// CheckC07Corpus executes a real loopback HTTP SSE probe against both beforePath
// (expected to terminate cleanly with error event + EOF) and afterPath (expected
// to hang when aclose() raises inside finally before queueing the sentinel).
func CheckC07Corpus(ctx context.Context, beforePath, afterPath string, timeout time.Duration) (C07CorpusResult, error) {
	if timeout <= 0 {
		timeout = defaultC07Timeout
	}
	pyBin, err := resolveStdlibPython()
	if err != nil {
		return C07CorpusResult{}, err
	}
	beforeAbs, err := filepath.Abs(beforePath)
	if err != nil {
		return C07CorpusResult{}, fmt.Errorf("resolve before path: %w", err)
	}
	afterAbs, err := filepath.Abs(afterPath)
	if err != nil {
		return C07CorpusResult{}, fmt.Errorf("resolve after path: %w", err)
	}
	if _, err := os.Stat(beforeAbs); err != nil {
		return C07CorpusResult{}, fmt.Errorf("before file not found: %w", err)
	}
	if _, err := os.Stat(afterAbs); err != nil {
		return C07CorpusResult{}, fmt.Errorf("after file not found: %w", err)
	}

	beforeOK, beforeDetail := evalC07Subject(ctx, pyBin, beforeAbs, timeout)
	afterOK, afterDetail := evalC07Subject(ctx, pyBin, afterAbs, timeout)

	res := C07CorpusResult{
		BeforePath:   beforePath,
		AfterPath:    afterPath,
		BeforePassed: beforeOK,
		AfterPassed:  afterOK,
		Detected:     beforeOK && !afterOK,
		BeforeDetail: beforeDetail,
		AfterDetail:  afterDetail,
	}
	if !afterOK {
		res.Signal = SignalC07SSEHang
	}
	return res, nil
}

func evalC07Subject(ctx context.Context, pyBin, moduleAbsPath string, timeout time.Duration) (bool, string) {
	tmpDir, err := os.MkdirTemp("", "alt-verifylive-c07-*")
	if err != nil {
		return false, fmt.Sprintf("mkdirtemp: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	scriptPath := filepath.Join(tmpDir, "c07_server.py")
	if err := os.WriteFile(scriptPath, []byte(c07CorpusServerPy), fsperm.File); err != nil {
		return false, fmt.Sprintf("write c07 script: %v", err)
	}
	readyFile := filepath.Join(tmpDir, "ready.url")
	child, err := startLoopbackChild(ctx, childSpec{
		Args:         []string{pyBin, scriptPath},
		Dir:          tmpDir,
		Env:          []string{"ALT_C07_MODULE=" + moduleAbsPath, "PYTHONUNBUFFERED=1"},
		ReadyFile:    readyFile,
		ReadyTimeout: 10 * time.Second,
	})
	if err != nil {
		return false, fmt.Sprintf("start c07 server: %v", err)
	}
	defer child.Stop()

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, child.baseURL+"/sse", nil)
	if err != nil {
		return false, err.Error()
	}
	client := loopbackHTTPClient(timeout)
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("SSE request failed/hung before headers: %v", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return false, fmt.Sprintf("SSE stream hung without terminal sentinel (partial=%q, err=%v)", strings.TrimSpace(string(body)), readErr)
	}
	bodyStr := string(body)
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("unexpected status %d", resp.StatusCode)
	}
	if !strings.Contains(bodyStr, `data: "chunk-1"`) || !strings.Contains(bodyStr, "event: error") {
		return false, fmt.Sprintf("missing expected SSE frames: %q", bodyStr)
	}
	return true, "received data chunk + error event + clean EOF before timeout"
}

// ============================================================================
// Probe 2: Body Limit (C-15) — real pre-processor Echo v5 on 127.0.0.1:0
// ============================================================================

const bodyLimitOverlayGo = `package bootstrap

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"pre-processor/internal/httpserver"
)

func TestVerifyLiveBodyLimitServer(t *testing.T) {
	deps := newTestHTTPServer(t, "verify-live")
	e := NewHTTPServer(deps, false, "")

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen 127.0.0.1:0: %v", err)
	}
	srv := httpserver.New(httpserver.Config{
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
	})
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	readyFile := os.Getenv("ALT_VERIFYLIVE_READY_FILE")
	stopFile := os.Getenv("ALT_VERIFYLIVE_STOP_FILE")
	tmpReady := readyFile + ".tmp"
	if err := os.WriteFile(tmpReady, []byte("http://"+ln.Addr().String()+"\n"), 0o600); err != nil {
		t.Fatalf("write ready file: %v", err)
	}
	if err := os.Rename(tmpReady, readyFile); err != nil {
		t.Fatalf("rename ready file: %v", err)
	}

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
waitLoop:
	for {
		if _, statErr := os.Stat(stopFile); statErr == nil {
			break waitLoop
		}
		select {
		case <-t.Context().Done():
			break waitLoop
		case <-deadline.C:
			break waitLoop
		case <-ticker.C:
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	<-errCh
}
`

func runBodyLimitProbe(ctx context.Context, repoRoot string) (res ProbeResult, logOut string) {
	start := time.Now()
	res = ProbeResult{
		Name:    "body-limit",
		Service: "pre-processor/app",
		OK:      true,
	}
	var logBuf bytes.Buffer
	defer func() {
		res.DurationMS = time.Since(start).Milliseconds()
	}()

	tmpDir, err := os.MkdirTemp("", "alt-verifylive-bodylimit-*")
	if err != nil {
		res.OK = false
		res.Error = fmt.Sprintf("mkdirtemp: %v", err)
		return res, res.Error + "\n"
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	targetAbs := filepath.Join(repoRoot, "pre-processor", "app", "bootstrap", "verifylive_probe_test.go")
	overlayPath, err := writeOverlay(tmpDir, targetAbs, "verifylive_probe_test.go", bodyLimitOverlayGo)
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, res.Error + "\n"
	}

	readyFile := filepath.Join(tmpDir, "ready.url")
	child, err := startLoopbackChild(ctx, childSpec{
		Args: []string{
			"go", "-C", filepath.Join(repoRoot, "pre-processor", "app"),
			"test", "-overlay", overlayPath, "-v", "-count=1", "-run", "^TestVerifyLiveBodyLimitServer$", "./bootstrap",
		},
		Dir:          repoRoot,
		Env:          []string{"MTLS_ALLOWED_PEERS="},
		ReadyFile:    readyFile,
		ReadyTimeout: 45 * time.Second,
	})
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, err.Error() + "\n"
	}
	res.BindAddr = child.baseURL
	fmt.Fprintf(&logBuf, "bind=%s\n", child.baseURL)

	client := loopbackHTTPClient(10 * time.Second)

	// 1. /health
	hReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, child.baseURL+"/health", nil)
	if resp, rerr := client.Do(hReq); rerr != nil {
		addCheck(&res, "health", false, rerr.Error())
	} else {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		ok := resp.StatusCode == http.StatusOK && strings.Contains(string(body), "healthy")
		addCheck(&res, "health", ok, fmt.Sprintf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body))))
		fmt.Fprintf(&logBuf, "GET /health -> %d %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// 2. Exactly 1,000,000 bytes (former Echo v4 "1M" decimal MB) must NOT return 413.
	// Use valid JSON with empty article_id so Echo's BodyLimit passes all 1,000,000 bytes
	// to HandleSummarize, which reads the full body and returns 400 (missing article_id).
	atLimit := exactSizeJSONPayload(1_000_000)
	reqAt, _ := http.NewRequestWithContext(ctx, http.MethodPost, child.baseURL+"/api/v1/summarize", bytes.NewReader(atLimit))
	reqAt.Header.Set("Content-Type", "application/json")
	reqAt.Header.Set("Expect", "100-continue")
	if resp, rerr := client.Do(reqAt); rerr != nil {
		addCheck(&res, "accepts-1000000-bytes", false, rerr.Error())
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		ok := resp.StatusCode != http.StatusRequestEntityTooLarge
		addCheck(&res, "accepts-1000000-bytes", ok, fmt.Sprintf("bytes=%d status=%d (want != 413)", len(atLimit), resp.StatusCode))
		fmt.Fprintf(&logBuf, "POST /api/v1/summarize (%d bytes) -> %d\n", len(atLimit), resp.StatusCode)
	}

	// 3. 1,000,001 bytes MUST return 413 Request Entity Too Large.
	overLimit := exactSizeJSONPayload(1_000_001)
	reqOver, _ := http.NewRequestWithContext(ctx, http.MethodPost, child.baseURL+"/api/v1/summarize", bytes.NewReader(overLimit))
	reqOver.Header.Set("Content-Type", "application/json")
	reqOver.Header.Set("Expect", "100-continue")
	if resp, rerr := client.Do(reqOver); rerr != nil {
		addCheck(&res, "rejects-1000001-bytes", false, rerr.Error())
	} else {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		ok := resp.StatusCode == http.StatusRequestEntityTooLarge
		addCheck(&res, "rejects-1000001-bytes", ok, fmt.Sprintf("bytes=%d status=%d (want 413)", len(overLimit), resp.StatusCode))
		fmt.Fprintf(&logBuf, "POST /api/v1/summarize (%d bytes) -> %d %s\n", len(overLimit), resp.StatusCode, strings.TrimSpace(string(body)))
	}

	childLogs := child.Stop()
	fmt.Fprintf(&logBuf, "--- child stdout/stderr ---\n%s\n", childLogs)
	return res, logBuf.String()
}

func exactSizeJSONPayload(totalBytes int) []byte {
	const prefix = `{"article_id":"","content":"`
	const suffix = `"}`
	padLen := totalBytes - len(prefix) - len(suffix)
	if padLen < 0 {
		padLen = 0
	}
	buf := make([]byte, 0, totalBytes)
	buf = append(buf, prefix...)
	buf = append(buf, bytes.Repeat([]byte("a"), padLen)...)
	buf = append(buf, suffix...)
	return buf
}

// ============================================================================
// Probe 3: Auth Hub — real auth-hub Echo v5 + local loopback Kratos stub
// ============================================================================

const authHubOverlayGo = `package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"auth-hub/config"
	"auth-hub/internal/adapter/gateway"
	adapterhandler "auth-hub/internal/adapter/handler"
	"auth-hub/internal/httpserver"
	infracache "auth-hub/internal/infrastructure/cache"
	infratoken "auth-hub/internal/infrastructure/token"
	"auth-hub/internal/usecase"
	appmiddleware "auth-hub/middleware"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func TestVerifyLiveAuthHubServer(t *testing.T) {
	kratosURL := os.Getenv("ALT_VERIFYLIVE_KRATOS_URL")
	if kratosURL == "" {
		t.Fatal("ALT_VERIFYLIVE_KRATOS_URL is required")
	}
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ephemeral ed25519 key: %v", err)
	}

	log := slog.New(slog.DiscardHandler)
	sessionCache := infracache.NewSessionCache(30 * time.Second)
	kratosGW := gateway.NewKratosGateway(kratosURL, kratosURL, 5*time.Second)
	jwtIssuer := infratoken.NewJWTIssuer(infratoken.JWTConfig{
		PrivateKey: privKey,
		Issuer:     "auth-hub",
		Audience:   "alt-backend",
		TTL:        5 * time.Minute,
	})

	validateUC := usecase.NewValidateSession(kratosGW, sessionCache, log)
	sessionUC := usecase.NewGetSession(kratosGW, sessionCache, jwtIssuer, log)
	invalidateUC := usecase.NewInvalidateSession(sessionCache, log)

	validateHandler := adapterhandler.NewValidateHandler(validateUC, jwtIssuer)
	sessionHandler := adapterhandler.NewSessionHandler(sessionUC, invalidateUC)
	healthHandler := adapterhandler.NewHealthHandler()

	e := echo.New()
	e.Logger = log
	configureTrustedProxies(context.Background(), e, &config.Config{TrustedProxiesDisabled: true}, log)
	e.Use(appmiddleware.SecurityHeaders())
	e.Use(newRequestLoggerMiddleware())
	e.Use(middleware.Recover())

	e.GET("/validate", validateHandler.Handle)
	e.GET("/session", sessionHandler.Handle)
	e.GET("/health", healthHandler.Handle)

	handler := buildPlaintextHandler(e, true)
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen 127.0.0.1:0: %v", err)
	}
	srv := httpserver.New(httpserver.Config{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	})
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	readyFile := os.Getenv("ALT_VERIFYLIVE_READY_FILE")
	stopFile := os.Getenv("ALT_VERIFYLIVE_STOP_FILE")
	tmpReady := readyFile + ".tmp"
	if err := os.WriteFile(tmpReady, []byte("http://"+ln.Addr().String()+"\n"), 0o600); err != nil {
		t.Fatalf("write ready file: %v", err)
	}
	if err := os.Rename(tmpReady, readyFile); err != nil {
		t.Fatalf("rename ready file: %v", err)
	}

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
waitLoop:
	for {
		if _, statErr := os.Stat(stopFile); statErr == nil {
			break waitLoop
		}
		select {
		case <-t.Context().Done():
			break waitLoop
		case <-deadline.C:
			break waitLoop
		case <-ticker.C:
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	<-errCh
}
`

func runAuthHubProbe(ctx context.Context, repoRoot string) (res ProbeResult, logOut string) {
	start := time.Now()
	res = ProbeResult{
		Name:    "auth-hub",
		Service: "auth-hub",
		OK:      true,
	}
	var logBuf bytes.Buffer
	defer func() {
		res.DurationMS = time.Since(start).Milliseconds()
	}()

	validSessionToken := "verify-live-valid-cookie"
	kratosStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions/whoami" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		cookieHeader := r.Header.Get("Cookie")
		if strings.Contains(cookieHeader, "ory_kratos_session="+validSessionToken) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": "sess-verify-live-01",
				"active": true,
				"identity": {
					"id": "user-verify-live-01",
					"schema_id": "default",
					"schema_url": "http://127.0.0.1/schema",
					"created_at": "2026-01-01T00:00:00Z",
					"traits": {
						"email": "verify@alt.local",
						"role": "user"
					}
				}
			}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"message":"no active session"}}`))
	}))
	defer kratosStub.Close()
	fmt.Fprintf(&logBuf, "kratos_stub=%s\n", kratosStub.URL)

	tmpDir, err := os.MkdirTemp("", "alt-verifylive-authhub-*")
	if err != nil {
		res.OK = false
		res.Error = fmt.Sprintf("mkdirtemp: %v", err)
		return res, res.Error + "\n"
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	targetAbs := filepath.Join(repoRoot, "auth-hub", "cmd", "auth-hub", "verifylive_probe_test.go")
	overlayPath, err := writeOverlay(tmpDir, targetAbs, "verifylive_probe_test.go", authHubOverlayGo)
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, res.Error + "\n"
	}

	readyFile := filepath.Join(tmpDir, "ready.url")
	child, err := startLoopbackChild(ctx, childSpec{
		Args: []string{
			"go", "-C", filepath.Join(repoRoot, "auth-hub"),
			"test", "-overlay", overlayPath, "-v", "-count=1", "-run", "^TestVerifyLiveAuthHubServer$", "./cmd/auth-hub",
		},
		Dir:          repoRoot,
		Env:          []string{"ALT_VERIFYLIVE_KRATOS_URL=" + kratosStub.URL},
		ReadyFile:    readyFile,
		ReadyTimeout: 45 * time.Second,
	})
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, err.Error() + "\n"
	}
	res.BindAddr = child.baseURL
	fmt.Fprintf(&logBuf, "bind=%s\n", child.baseURL)

	client := loopbackHTTPClient(5 * time.Second)

	// 1. /health -> 200
	hReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, child.baseURL+"/health", nil)
	if resp, rerr := client.Do(hReq); rerr != nil {
		addCheck(&res, "health", false, rerr.Error())
	} else {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		ok := resp.StatusCode == http.StatusOK && strings.Contains(string(body), "healthy")
		addCheck(&res, "health", ok, fmt.Sprintf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body))))
		fmt.Fprintf(&logBuf, "GET /health -> %d %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// 2. /validate without cookie -> 401 and no X-Alt-* headers
	uReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, child.baseURL+"/validate", nil)
	if resp, rerr := client.Do(uReq); rerr != nil {
		addCheck(&res, "validate-unauthenticated-401", false, rerr.Error())
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		ok := resp.StatusCode == http.StatusUnauthorized && resp.Header.Get("X-Alt-User-Id") == "" && resp.Header.Get("X-Alt-Backend-Token") == ""
		addCheck(&res, "validate-unauthenticated-401", ok, fmt.Sprintf("status=%d x_alt_user_id=%q", resp.StatusCode, resp.Header.Get("X-Alt-User-Id")))
		fmt.Fprintf(&logBuf, "GET /validate (no cookie) -> %d\n", resp.StatusCode)
	}

	// 3. /validate with invalid cookie -> 401
	badReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, child.baseURL+"/validate", nil)
	badReq.Header.Set("Cookie", "ory_kratos_session=expired-invalid-cookie")
	if resp, rerr := client.Do(badReq); rerr != nil {
		addCheck(&res, "validate-invalid-cookie-401", false, rerr.Error())
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		ok := resp.StatusCode == http.StatusUnauthorized && resp.Header.Get("X-Alt-User-Id") == ""
		addCheck(&res, "validate-invalid-cookie-401", ok, fmt.Sprintf("status=%d", resp.StatusCode))
		fmt.Fprintf(&logBuf, "GET /validate (invalid cookie) -> %d\n", resp.StatusCode)
	}

	// 4. /validate with valid cookie -> 200 + authoritative X-Alt-* headers + JWT
	okReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, child.baseURL+"/validate", nil)
	okReq.Header.Set("Cookie", "ory_kratos_session="+validSessionToken)
	if resp, rerr := client.Do(okReq); rerr != nil {
		addCheck(&res, "validate-authenticated-200", false, rerr.Error())
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		userID := resp.Header.Get("X-Alt-User-Id")
		tenantID := resp.Header.Get("X-Alt-Tenant-Id")
		email := resp.Header.Get("X-Alt-User-Email")
		jwtSegs := len(strings.Split(resp.Header.Get("X-Alt-Backend-Token"), "."))
		ok := resp.StatusCode == http.StatusOK &&
			userID == "user-verify-live-01" &&
			tenantID == "user-verify-live-01" &&
			email == "verify@alt.local" &&
			jwtSegs == 3
		addCheck(&res, "validate-authenticated-200", ok, fmt.Sprintf("status=%d X-Alt-User-Id=%q X-Alt-Tenant-Id=%q X-Alt-User-Email=%q jwt_segments=%d",
			resp.StatusCode, userID, tenantID, email, jwtSegs))
		fmt.Fprintf(&logBuf, "GET /validate (valid cookie) -> %d user=%s tenant=%s email=%s jwt_segments=%d\n",
			resp.StatusCode, userID, tenantID, email, jwtSegs)
	}

	// 5. /session with valid cookie -> 200 JSON + JWT header
	sessReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, child.baseURL+"/session", nil)
	sessReq.Header.Set("Cookie", "ory_kratos_session="+validSessionToken)
	if resp, rerr := client.Do(sessReq); rerr != nil {
		addCheck(&res, "session-authenticated-200", false, rerr.Error())
	} else {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var parsed struct {
			OK   bool `json:"ok"`
			User struct {
				ID       string `json:"id"`
				TenantID string `json:"tenantId"`
				Email    string `json:"email"`
			} `json:"user"`
			Session struct {
				ID     string `json:"id"`
				Active bool   `json:"active"`
			} `json:"session"`
		}
		jerr := json.Unmarshal(body, &parsed)
		jwtSegs := len(strings.Split(resp.Header.Get("X-Alt-Backend-Token"), "."))
		ok := jerr == nil &&
			resp.StatusCode == http.StatusOK &&
			parsed.OK &&
			parsed.User.ID == "user-verify-live-01" &&
			parsed.User.TenantID == "user-verify-live-01" &&
			parsed.Session.Active &&
			jwtSegs == 3
		addCheck(&res, "session-authenticated-200", ok, fmt.Sprintf("status=%d ok=%v user=%q active=%v jwt_segments=%d",
			resp.StatusCode, parsed.OK, parsed.User.ID, parsed.Session.Active, jwtSegs))
		fmt.Fprintf(&logBuf, "GET /session (valid cookie) -> %d body=%s jwt_segments=%d\n",
			resp.StatusCode, strings.TrimSpace(string(body)), jwtSegs)
	}

	childLogs := child.Stop()
	fmt.Fprintf(&logBuf, "--- child stdout/stderr ---\n%s\n", childLogs)
	return res, logBuf.String()
}

// ============================================================================
// Probe 4: Inference Proxy (C-14) — real docker/inference-proxy on 127.0.0.1:0
// ============================================================================

const inferenceProxyOverlayGo = `package main

import (
	"context"
	"net"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestVerifyLiveInferenceProxyServer(t *testing.T) {
	upstreamRaw := os.Getenv("ALT_VERIFYLIVE_UPSTREAM_URL")
	token := os.Getenv("ALT_VERIFYLIVE_BEARER_TOKEN")
	if upstreamRaw == "" || token == "" {
		t.Fatal("ALT_VERIFYLIVE_UPSTREAM_URL and ALT_VERIFYLIVE_BEARER_TOKEN are required")
	}
	targetURL, err := url.Parse(upstreamRaw)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}
	handler := NewProxyHandler(targetURL, token, 5*time.Second, DefaultMaxRequestBodyBytes)
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen 127.0.0.1:0: %v", err)
	}
	srv := NewServer(ln.Addr().String(), handler, 5*time.Second)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	readyFile := os.Getenv("ALT_VERIFYLIVE_READY_FILE")
	stopFile := os.Getenv("ALT_VERIFYLIVE_STOP_FILE")
	tmpReady := readyFile + ".tmp"
	if err := os.WriteFile(tmpReady, []byte("http://"+ln.Addr().String()+"\n"), 0o600); err != nil {
		t.Fatalf("write ready file: %v", err)
	}
	if err := os.Rename(tmpReady, readyFile); err != nil {
		t.Fatalf("rename ready file: %v", err)
	}

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
waitLoop:
	for {
		if _, statErr := os.Stat(stopFile); statErr == nil {
			break waitLoop
		}
		select {
		case <-t.Context().Done():
			break waitLoop
		case <-deadline.C:
			break waitLoop
		case <-ticker.C:
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	<-errCh
}
`

func ephemeralToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "verify-live-" + hex.EncodeToString(buf), nil
}

func runInferenceProxyProbe(ctx context.Context, repoRoot string) (res ProbeResult, logOut string) {
	start := time.Now()
	res = ProbeResult{
		Name:    "inference-proxy",
		Service: "docker/inference-proxy",
		OK:      true,
	}
	var logBuf bytes.Buffer
	defer func() {
		res.DurationMS = time.Since(start).Milliseconds()
	}()

	token, err := ephemeralToken()
	if err != nil {
		res.OK = false
		res.Error = fmt.Sprintf("generate ephemeral token: %v", err)
		return res, res.Error + "\n"
	}

	type echoCapture struct {
		Host            string `json:"host"`
		XForwardedFor   string `json:"x_forwarded_for"`
		XForwardedHost  string `json:"x_forwarded_host"`
		XForwardedProto string `json:"x_forwarded_proto"`
		PathAndQuery    string `json:"path_and_query"`
		CustomHeader    string `json:"custom_header"`
		Authorization   string `json:"authorization"`
	}
	var upstreamHits atomic.Int64
	upstreamStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		cap := echoCapture{
			Host:            r.Host,
			XForwardedFor:   r.Header.Get("X-Forwarded-For"),
			XForwardedHost:  r.Header.Get("X-Forwarded-Host"),
			XForwardedProto: r.Header.Get("X-Forwarded-Proto"),
			PathAndQuery:    r.URL.RequestURI(),
			CustomHeader:    r.Header.Get("X-Custom-Header"),
			Authorization:   r.Header.Get("Authorization"),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cap)
	}))
	defer upstreamStub.Close()
	upstreamHost := strings.TrimPrefix(upstreamStub.URL, "http://")
	fmt.Fprintf(&logBuf, "upstream_stub=%s\n", upstreamStub.URL)

	tmpDir, err := os.MkdirTemp("", "alt-verifylive-infproxy-*")
	if err != nil {
		res.OK = false
		res.Error = fmt.Sprintf("mkdirtemp: %v", err)
		return res, res.Error + "\n"
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	targetAbs := filepath.Join(repoRoot, "docker", "inference-proxy", "verifylive_probe_test.go")
	overlayPath, err := writeOverlay(tmpDir, targetAbs, "verifylive_probe_test.go", inferenceProxyOverlayGo)
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, res.Error + "\n"
	}

	readyFile := filepath.Join(tmpDir, "ready.url")
	child, err := startLoopbackChild(ctx, childSpec{
		Args: []string{
			"go", "-C", filepath.Join(repoRoot, "docker", "inference-proxy"),
			"test", "-overlay", overlayPath, "-v", "-count=1", "-run", "^TestVerifyLiveInferenceProxyServer$", ".",
		},
		Dir: repoRoot,
		Env: []string{
			"ALT_VERIFYLIVE_UPSTREAM_URL=" + upstreamStub.URL,
			"ALT_VERIFYLIVE_BEARER_TOKEN=" + token,
		},
		ReadyFile:    readyFile,
		ReadyTimeout: 45 * time.Second,
	})
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res, err.Error() + "\n"
	}
	res.BindAddr = child.baseURL
	fmt.Fprintf(&logBuf, "bind=%s\n", child.baseURL)

	client := loopbackHTTPClient(5 * time.Second)

	// 1. /health -> 200 without auth, 0 upstream hits
	hReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, child.baseURL+"/health", nil)
	if resp, rerr := client.Do(hReq); rerr != nil {
		addCheck(&res, "health", false, rerr.Error())
	} else {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		ok := resp.StatusCode == http.StatusOK && upstreamHits.Load() == 0
		addCheck(&res, "health", ok, fmt.Sprintf("status=%d body=%q upstream_hits=%d", resp.StatusCode, strings.TrimSpace(string(body)), upstreamHits.Load()))
		fmt.Fprintf(&logBuf, "GET /health -> %d upstream_hits=%d\n", resp.StatusCode, upstreamHits.Load())
	}

	// 2. Unauthenticated request -> 401, 0 upstream hits
	noAuthReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, child.baseURL+"/api/generate", strings.NewReader(`{}`))
	if resp, rerr := client.Do(noAuthReq); rerr != nil {
		addCheck(&res, "unauthenticated-401", false, rerr.Error())
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		ok := resp.StatusCode == http.StatusUnauthorized && upstreamHits.Load() == 0
		addCheck(&res, "unauthenticated-401", ok, fmt.Sprintf("status=%d upstream_hits=%d", resp.StatusCode, upstreamHits.Load()))
		fmt.Fprintf(&logBuf, "POST /api/generate (no auth) -> %d upstream_hits=%d\n", resp.StatusCode, upstreamHits.Load())
	}

	// 3. Disallowed admin route -> 403, 0 upstream hits
	denyReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, child.baseURL+"/api/pull", strings.NewReader(`{}`))
	denyReq.Header.Set("Authorization", "Bearer "+token)
	if resp, rerr := client.Do(denyReq); rerr != nil {
		addCheck(&res, "disallowed-route-403", false, rerr.Error())
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		ok := resp.StatusCode == http.StatusForbidden && upstreamHits.Load() == 0
		addCheck(&res, "disallowed-route-403", ok, fmt.Sprintf("status=%d upstream_hits=%d", resp.StatusCode, upstreamHits.Load()))
		fmt.Fprintf(&logBuf, "POST /api/pull (disallowed) -> %d upstream_hits=%d\n", resp.StatusCode, upstreamHits.Load())
	}

	// 4. Valid forwarded request -> verify all 6+ upstream header invariants (C-14)
	fwdReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, child.baseURL+"/api/generate?stream=true&model=qwen", strings.NewReader(`{}`))
	fwdReq.Header.Set("Authorization", "Bearer "+token)
	fwdReq.Header.Set("X-Forwarded-For", "203.0.113.10")
	fwdReq.Header.Set("X-Custom-Header", "verify-live-c14")
	if resp, rerr := client.Do(fwdReq); rerr != nil {
		addCheck(&res, "c14-forwarded-headers", false, rerr.Error())
	} else {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var got echoCapture
		jerr := json.Unmarshal(body, &got)
		fmt.Fprintf(&logBuf, "POST /api/generate?stream=true&model=qwen -> %d upstream_echo=%s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		if jerr != nil || resp.StatusCode != http.StatusOK {
			addCheck(&res, "c14-forwarded-headers", false, fmt.Sprintf("status=%d decode_err=%v", resp.StatusCode, jerr))
		} else {
			addCheck(&res, "header-host", got.Host == upstreamHost, fmt.Sprintf("got=%q want=%q", got.Host, upstreamHost))
			addCheck(&res, "header-x-forwarded-for", got.XForwardedFor == "203.0.113.10, 127.0.0.1", fmt.Sprintf("got=%q want=%q", got.XForwardedFor, "203.0.113.10, 127.0.0.1"))
			addCheck(&res, "header-x-forwarded-host-not-synthesized", got.XForwardedHost == "", fmt.Sprintf("got=%q want empty", got.XForwardedHost))
			addCheck(&res, "header-x-forwarded-proto-not-synthesized", got.XForwardedProto == "", fmt.Sprintf("got=%q want empty", got.XForwardedProto))
			addCheck(&res, "path-and-query-preserved", got.PathAndQuery == "/api/generate?stream=true&model=qwen", fmt.Sprintf("got=%q", got.PathAndQuery))
			addCheck(&res, "custom-header-preserved", got.CustomHeader == "verify-live-c14", fmt.Sprintf("got=%q", got.CustomHeader))
			addCheck(&res, "authorization-stripped", got.Authorization == "", fmt.Sprintf("got=%q want empty", got.Authorization))
		}
	}

	childLogs := child.Stop()
	fmt.Fprintf(&logBuf, "--- child stdout/stderr ---\n%s\n", childLogs)
	return res, logBuf.String()
}
