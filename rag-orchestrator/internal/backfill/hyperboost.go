package backfill

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

const (
	hyperBoostContainerName = "backfill-hyperboost"
	hyperBoostPort          = 11434 // Internal Ollama port
	hyperBoostHostPort      = 11437 // Host port for health checks
	hyperBoostImage         = "ollama/ollama:latest"
	hyperBoostModel         = "embeddinggemma"
	hyperBoostNetwork       = "alt_alt-network" // Same network as orchestrator
)

// HyperBoost manages a temporary Ollama container for local GPU embedding.
type HyperBoost struct {
	containerID string
	port        int
	model       string
	logger      *slog.Logger
}

// NewHyperBoost creates a new HyperBoost instance.
func NewHyperBoost(logger *slog.Logger) (*HyperBoost, error) {
	return &HyperBoost{
		port:   hyperBoostPort,
		model:  hyperBoostModel,
		logger: logger,
	}, nil
}

// Start creates and starts the Ollama container with GPU access.
func (h *HyperBoost) Start(ctx context.Context) error {
	h.logger.Info("starting hyper-boost container",
		slog.String("image", hyperBoostImage),
		slog.Int("port", h.port),
	)

	// Check if container already exists and remove it
	checkCmd := exec.CommandContext(ctx, "docker", "ps", "-aq", "-f", fmt.Sprintf("name=%s", hyperBoostContainerName))
	if output, err := checkCmd.Output(); err == nil && len(bytes.TrimSpace(output)) > 0 {
		h.logger.Info("removing existing container")
		rmCmd := exec.CommandContext(ctx, "docker", "rm", "-f", hyperBoostContainerName)
		_ = rmCmd.Run()
	}

	// Start the container with full NVIDIA GPU support
	// Connect to the same network as orchestrator so it can be accessed by container name
	args := []string{
		"run", "--rm", "-d",
		"--name", hyperBoostContainerName,
		"--network", hyperBoostNetwork,
		"--gpus", "all",
		"-p", fmt.Sprintf("%d:11434", hyperBoostHostPort), // Host port for health checks
		"-e", "NVIDIA_VISIBLE_DEVICES=all",
		"-e", "NVIDIA_DRIVER_CAPABILITIES=compute,utility",
		"-e", "OLLAMA_NUM_PARALLEL=8", // Enable parallel inference
		hyperBoostImage,
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start container: %w (stderr: %s)", err, stderr.String())
	}

	h.containerID = strings.TrimSpace(stdout.String())
	h.logger.Info("hyper-boost container started",
		slog.String("container_id", h.containerID[:12]),
	)

	return nil
}

// WaitReady waits for the Ollama server to be ready.
func (h *HyperBoost) WaitReady(ctx context.Context) error {
	h.logger.Info("waiting for hyper-boost to be ready")

	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://localhost:%d/api/tags", hyperBoostHostPort)

	for i := 0; i < 60; i++ { // Wait up to 60 seconds
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				h.logger.Info("hyper-boost is ready")
				return nil
			}
		}

		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("hyper-boost not ready after 60 seconds")
}

// PullModel pulls the embedding model.
func (h *HyperBoost) PullModel(ctx context.Context) error {
	h.logger.Info("pulling embedding model", slog.String("model", h.model))

	client := &http.Client{Timeout: 10 * time.Minute}
	url := fmt.Sprintf("http://localhost:%d/api/pull", hyperBoostHostPort)

	reqBody := fmt.Sprintf(`{"name": "%s"}`, h.model)
	resp, err := client.Post(url, "application/json", strings.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("pull model request: %w", err)
	}
	defer resp.Body.Close()

	// Drain the response (model pull streams progress)
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("drain model pull response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pull model failed with status %d", resp.StatusCode)
	}

	h.logger.Info("embedding model ready", slog.String("model", h.model))
	return nil
}

// Stop stops and removes the container.
func (h *HyperBoost) Stop(ctx context.Context) error {
	if h.containerID == "" {
		return nil
	}

	h.logger.Info("stopping hyper-boost container")

	cmd := exec.CommandContext(ctx, "docker", "stop", hyperBoostContainerName)
	if err := cmd.Run(); err != nil {
		h.logger.Warn("failed to stop container", slog.String("error", err.Error()))
	}

	h.logger.Info("hyper-boost container stopped")
	return nil
}

// EmbedderURL returns the URL for the embedder.
// Returns container name URL for access from within Docker network.
func (h *HyperBoost) EmbedderURL() string {
	return fmt.Sprintf("http://%s:%d", hyperBoostContainerName, hyperBoostPort)
}

// Close releases resources.
func (h *HyperBoost) Close() error {
	return nil
}
