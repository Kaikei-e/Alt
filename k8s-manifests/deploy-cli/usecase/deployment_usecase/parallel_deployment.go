package deployment_usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// ParallelDeploymentConfig holds configuration for parallel deployment
type ParallelDeploymentConfig struct {
	MaxConcurrency int
	ChunkSize      int
	RetryAttempts  int
	RetryDelay     time.Duration
}

// DefaultParallelConfig returns default parallel deployment configuration
func DefaultParallelConfig() *ParallelDeploymentConfig {
	return &ParallelDeploymentConfig{
		MaxConcurrency: 3, // Deploy up to 3 charts concurrently
		ChunkSize:      5, // Process charts in chunks of 5
		RetryAttempts:  2,
		RetryDelay:     time.Second * 5,
	}
}

// ParallelChartDeployer handles parallel chart deployment operations
type ParallelChartDeployer struct {
	logger logger_port.LoggerPort
	config *ParallelDeploymentConfig
}

// NewParallelChartDeployer creates a new parallel chart deployer
func NewParallelChartDeployer(logger logger_port.LoggerPort, config *ParallelDeploymentConfig) *ParallelChartDeployer {
	if config == nil {
		config = DefaultParallelConfig()
	}
	return &ParallelChartDeployer{
		logger: logger,
		config: config,
	}
}

// ChartDeployJob represents a single chart deployment job
type ChartDeployJob struct {
	Chart   domain.Chart
	Options *domain.DeploymentOptions
	Result  chan domain.DeploymentResult
}

// ChartWorkerPool manages a pool of workers for chart deployment
type ChartWorkerPool struct {
	workers  int
	jobs     chan ChartDeployJob
	results  chan domain.DeploymentResult
	deployer func(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) domain.DeploymentResult
	logger   logger_port.LoggerPort
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewChartWorkerPool creates a new chart worker pool
func NewChartWorkerPool(workers int, deployer func(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) domain.DeploymentResult, logger logger_port.LoggerPort) *ChartWorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &ChartWorkerPool{
		workers:  workers,
		jobs:     make(chan ChartDeployJob, workers*2),
		results:  make(chan domain.DeploymentResult, workers*2),
		deployer: deployer,
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the worker pool
func (p *ChartWorkerPool) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// Stop gracefully stops the worker pool
func (p *ChartWorkerPool) Stop() {
	close(p.jobs)
	p.wg.Wait()
	close(p.results)
	p.cancel()
}

// SubmitJob submits a chart deployment job
func (p *ChartWorkerPool) SubmitJob(job ChartDeployJob) {
	select {
	case p.jobs <- job:
	case <-p.ctx.Done():
		job.Result <- domain.DeploymentResult{
			ChartName: job.Chart.Name,
			Status:    domain.DeploymentStatusFailed,
			Error:     fmt.Errorf("worker pool stopped"),
			Duration:  0,
		}
	}
}

// worker processes chart deployment jobs
func (p *ChartWorkerPool) worker(id int) {
	defer p.wg.Done()

	p.logger.DebugWithContext("chart deployment worker started", map[string]interface{}{
		"worker_id": id,
	})

	for job := range p.jobs {
		select {
		case <-p.ctx.Done():
			job.Result <- domain.DeploymentResult{
				ChartName: job.Chart.Name,
				Status:    domain.DeploymentStatusFailed,
				Error:     fmt.Errorf("worker context cancelled"),
				Duration:  0,
			}
			return
		default:
			p.logger.DebugWithContext("worker processing chart", map[string]interface{}{
				"worker_id": id,
				"chart":     job.Chart.Name,
			})

			result := p.deployer(p.ctx, job.Chart, job.Options)

			select {
			case job.Result <- result:
			case <-p.ctx.Done():
				return
			}
		}
	}

	p.logger.DebugWithContext("chart deployment worker stopped", map[string]interface{}{
		"worker_id": id,
	})
}

// deployChartsParallel deploys charts in parallel within dependency constraints
func (d *ParallelChartDeployer) deployChartsParallel(ctx context.Context, groupName string, charts []domain.Chart, options *domain.DeploymentOptions, deploySingleChart func(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) domain.DeploymentResult) ([]domain.DeploymentResult, error) {

	d.logger.InfoWithContext("starting parallel chart deployment", map[string]interface{}{
		"group":           groupName,
		"chart_count":     len(charts),
		"max_concurrency": d.config.MaxConcurrency,
	})

	if len(charts) == 0 {
		return []domain.DeploymentResult{}, nil
	}

	// For simplicity, we'll deploy all charts in parallel within the same group
	// In a more sophisticated implementation, we'd analyze dependencies
	return d.deployChartBatch(ctx, groupName, charts, options, deploySingleChart)
}

// deployChartBatch deploys a batch of charts concurrently
func (d *ParallelChartDeployer) deployChartBatch(ctx context.Context, groupName string, charts []domain.Chart, options *domain.DeploymentOptions, deploySingleChart func(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) domain.DeploymentResult) ([]domain.DeploymentResult, error) {

	// Create worker pool
	pool := NewChartWorkerPool(d.config.MaxConcurrency, deploySingleChart, d.logger)
	pool.Start()
	defer pool.Stop()

	// Submit all jobs
	results := make([]domain.DeploymentResult, len(charts))
	resultChans := make([]chan domain.DeploymentResult, len(charts))

	for i, chart := range charts {
		resultChans[i] = make(chan domain.DeploymentResult, 1)
		job := ChartDeployJob{
			Chart:   chart,
			Options: options,
			Result:  resultChans[i],
		}
		pool.SubmitJob(job)
	}

	// Collect results
	for i := range charts {
		select {
		case result := <-resultChans[i]:
			results[i] = result
			d.logger.InfoWithContext("chart deployment completed", map[string]interface{}{
				"group":    groupName,
				"chart":    result.ChartName,
				"status":   result.Status,
				"duration": result.Duration,
			})
		case <-ctx.Done():
			return results, ctx.Err()
		}
	}

	d.logger.InfoWithContext("parallel chart deployment batch completed", map[string]interface{}{
		"group":       groupName,
		"chart_count": len(charts),
	})

	return results, nil
}

// CanDeployInParallel determines if charts can be deployed in parallel
func (d *ParallelChartDeployer) CanDeployInParallel(charts []domain.Chart) bool {
	// For now, we assume charts within the same deployment group can be deployed in parallel
	// In a more sophisticated implementation, we'd analyze:
	// 1. Chart dependencies
	// 2. Resource dependencies (e.g., databases must be ready before apps)
	// 3. Custom deployment order annotations
	return len(charts) > 1
}

// EstimateDeploymentTime estimates deployment time based on historical data
func (d *ParallelChartDeployer) EstimateDeploymentTime(charts []domain.Chart) time.Duration {
	// Simple estimation: base time per chart + overhead
	baseTimePerChart := time.Second * 30
	parallelOverhead := time.Second * 10

	if len(charts) <= d.config.MaxConcurrency {
		// All charts can run in parallel
		return baseTimePerChart + parallelOverhead
	}

	// Charts will be processed in batches
	batches := (len(charts) + d.config.MaxConcurrency - 1) / d.config.MaxConcurrency
	return time.Duration(batches) * (baseTimePerChart + parallelOverhead)
}
