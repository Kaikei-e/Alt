// Phase R4: 統合アダプター - 既存システムとの統合・後方互換性保証
package infrastructure

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"reflect"

	"deploy-cli/infrastructure/config"
	"deploy-cli/infrastructure/container"
	"deploy-cli/infrastructure/logging"
	"deploy-cli/utils/logger"
)

// InfrastructureContainer is the main container for all infrastructure components
type InfrastructureContainer struct {
	diContainer     *container.DependencyContainer
	serviceRegistry *container.ServiceRegistry
	factoryRegistry *container.FactoryRegistry
	configManager   *config.ConfigManager
	envConfig       *config.EnvironmentConfig
	structLogger    *logging.StructuredLogger
	contextManager  *logging.LogContextManager
	aggregator      *logging.LogAggregator
}

// NewInfrastructureContainer creates a new infrastructure container
func NewInfrastructureContainer(environment config.Environment) (*InfrastructureContainer, error) {
	// Create DI container
	diContainer := container.NewDependencyContainer()
	serviceRegistry := container.NewServiceRegistry(diContainer)
	factoryRegistry := container.NewFactoryRegistry(diContainer)

	// Create configuration management
	envConfig, err := config.NewEnvironmentConfig(environment)
	if err != nil {
		return nil, fmt.Errorf("failed to create environment config: %w", err)
	}

	if err := envConfig.LoadConfig(); err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Create logging infrastructure
	logConfig := convertToLoggerConfig(envConfig.GetLoggingConfig())
	structLogger := logging.NewStructuredLogger(logConfig)
	contextManager := logging.NewLogContextManager(structLogger)

	// Create log aggregator
	aggConfig := convertToAggregatorConfig(envConfig.GetLoggingConfig())
	aggregator := logging.NewLogAggregator(aggConfig)

	// Start aggregator
	if err := aggregator.Start(); err != nil {
		return nil, fmt.Errorf("failed to start log aggregator: %w", err)
	}

	infra := &InfrastructureContainer{
		diContainer:     diContainer,
		serviceRegistry: serviceRegistry,
		factoryRegistry: factoryRegistry,
		configManager:   envConfig.GetManager(),
		envConfig:       envConfig,
		structLogger:    structLogger,
		contextManager:  contextManager,
		aggregator:      aggregator,
	}

	// Register core services
	if err := infra.registerCoreServices(); err != nil {
		return nil, fmt.Errorf("failed to register core services: %w", err)
	}

	return infra, nil
}

// registerCoreServices registers core infrastructure services
func (ic *InfrastructureContainer) registerCoreServices() error {
	// Register config manager
	if err := ic.serviceRegistry.RegisterService(&container.ServiceRegistrationOptions{
		ServiceType: (*config.ConfigManager)(nil),
		Instance:    ic.configManager,
		Lifecycle:   container.Singleton,
		Description: "Configuration manager",
	}); err != nil {
		return fmt.Errorf("failed to register config manager: %w", err)
	}

	// Register environment config
	if err := ic.serviceRegistry.RegisterService(&container.ServiceRegistrationOptions{
		ServiceType: (*config.EnvironmentConfig)(nil),
		Instance:    ic.envConfig,
		Lifecycle:   container.Singleton,
		Description: "Environment-specific configuration",
	}); err != nil {
		return fmt.Errorf("failed to register environment config: %w", err)
	}

	// Register structured logger
	if err := ic.serviceRegistry.RegisterService(&container.ServiceRegistrationOptions{
		ServiceType: (*logging.StructuredLogger)(nil),
		Instance:    ic.structLogger,
		Lifecycle:   container.Singleton,
		Description: "Structured logger",
	}); err != nil {
		return fmt.Errorf("failed to register structured logger: %w", err)
	}

	// Register context manager
	if err := ic.serviceRegistry.RegisterService(&container.ServiceRegistrationOptions{
		ServiceType: (*logging.LogContextManager)(nil),
		Instance:    ic.contextManager,
		Lifecycle:   container.Singleton,
		Description: "Log context manager",
	}); err != nil {
		return fmt.Errorf("failed to register context manager: %w", err)
	}

	return nil
}

// GetDependencyContainer returns the DI container
func (ic *InfrastructureContainer) GetDependencyContainer() *container.DependencyContainer {
	return ic.diContainer
}

// GetServiceRegistry returns the service registry
func (ic *InfrastructureContainer) GetServiceRegistry() *container.ServiceRegistry {
	return ic.serviceRegistry
}

// GetFactoryRegistry returns the factory registry
func (ic *InfrastructureContainer) GetFactoryRegistry() *container.FactoryRegistry {
	return ic.factoryRegistry
}

// GetConfigManager returns the config manager
func (ic *InfrastructureContainer) GetConfigManager() *config.ConfigManager {
	return ic.configManager
}

// GetEnvironmentConfig returns the environment config
func (ic *InfrastructureContainer) GetEnvironmentConfig() *config.EnvironmentConfig {
	return ic.envConfig
}

// GetStructuredLogger returns the structured logger
func (ic *InfrastructureContainer) GetStructuredLogger() *logging.StructuredLogger {
	return ic.structLogger
}

// GetLogContextManager returns the log context manager
func (ic *InfrastructureContainer) GetLogContextManager() *logging.LogContextManager {
	return ic.contextManager
}

// GetLogAggregator returns the log aggregator
func (ic *InfrastructureContainer) GetLogAggregator() *logging.LogAggregator {
	return ic.aggregator
}

// CreateLegacyLoggerAdapter creates an adapter for existing logger.Logger
func (ic *InfrastructureContainer) CreateLegacyLoggerAdapter() *logger.Logger {
	return &logger.Logger{
		Logger: ic.createSlogAdapter(),
	}
}

// createSlogAdapter creates an slog adapter for structured logger
func (ic *InfrastructureContainer) createSlogAdapter() *slog.Logger {
	// Create handler that adapts to structured logger
	handler := &StructuredLoggerHandler{
		structLogger: ic.structLogger,
		level:        slog.Level(ic.structLogger.GetLevel()),
	}
	
	return slog.New(handler)
}

// StructuredLoggerHandler adapts StructuredLogger to slog.Handler interface
type StructuredLoggerHandler struct {
	structLogger *logging.StructuredLogger
	level        slog.Level
}

// Enabled implements slog.Handler
func (h *StructuredLoggerHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle implements slog.Handler
func (h *StructuredLoggerHandler) Handle(ctx context.Context, record slog.Record) error {
	// Convert slog.Record to our format
	attrs := make([]interface{}, 0, record.NumAttrs()*2)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr.Key, attr.Value.Any())
		return true
	})

	logLevel := logging.LogLevel(record.Level)
	h.structLogger.LogWithLevel(ctx, logLevel, record.Message, attrs...)
	return nil
}

// WithAttrs implements slog.Handler
func (h *StructuredLoggerHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	fields := make(map[string]interface{})
	for _, attr := range attrs {
		fields[attr.Key] = attr.Value.Any()
	}
	
	return &StructuredLoggerHandler{
		structLogger: h.structLogger.WithFields(fields),
		level:        h.level,
	}
}

// WithGroup implements slog.Handler
func (h *StructuredLoggerHandler) WithGroup(name string) slog.Handler {
	// For simplicity, we'll just add the group name as a field
	return &StructuredLoggerHandler{
		structLogger: h.structLogger.WithField("group", name),
		level:        h.level,
	}
}

// RegisterLegacyServices registers services for legacy code compatibility
func (ic *InfrastructureContainer) RegisterLegacyServices() error {
	// Register legacy logger
	legacyLogger := ic.CreateLegacyLoggerAdapter()
	if err := ic.serviceRegistry.RegisterService(&container.ServiceRegistrationOptions{
		ServiceType: (*logger.Logger)(nil),
		Instance:    legacyLogger,
		Lifecycle:   container.Singleton,
		Description: "Legacy logger adapter",
	}); err != nil {
		return fmt.Errorf("failed to register legacy logger: %w", err)
	}

	return nil
}

// HealthCheck performs health check on all infrastructure components
func (ic *InfrastructureContainer) HealthCheck(ctx context.Context) error {
	// Check service registry
	if err := ic.serviceRegistry.HealthCheck(ctx); err != nil {
		return fmt.Errorf("service registry health check failed: %w", err)
	}

	// Check configuration
	if _, exists := ic.configManager.Get("logging.level"); !exists {
		return fmt.Errorf("configuration health check failed: logging.level not found")
	}

	// Check structured logger
	if !ic.structLogger.IsLevelEnabled(logging.InfoLevel) {
		return fmt.Errorf("structured logger health check failed: info level not enabled")
	}

	return nil
}

// GetMetrics returns metrics for all infrastructure components
func (ic *InfrastructureContainer) GetMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})

	// Service registry metrics
	metrics["service_registry"] = ic.serviceRegistry.GetMetrics()

	// Context manager metrics
	metrics["log_context"] = ic.contextManager.GetMetrics()

	// Aggregator metrics
	metrics["log_aggregator"] = ic.aggregator.GetMetrics()

	// Configuration info
	metrics["configuration"] = ic.configManager.GetConfigInfo()

	return metrics
}

// Shutdown gracefully shuts down all infrastructure components
func (ic *InfrastructureContainer) Shutdown(ctx context.Context) error {
	// Stop log aggregator
	if err := ic.aggregator.Stop(); err != nil {
		ic.structLogger.Error(ctx, "Failed to stop log aggregator", "error", err)
	}

	// Close structured logger
	if err := ic.structLogger.Close(); err != nil {
		return fmt.Errorf("failed to close structured logger: %w", err)
	}

	// Dispose service registry
	if err := ic.serviceRegistry.Dispose(); err != nil {
		return fmt.Errorf("failed to dispose service registry: %w", err)
	}

	return nil
}

// Helper functions for config conversion

func convertToLoggerConfig(logConfig *config.LoggingConfig) *logging.LoggerConfig {
	var level logging.LogLevel
	switch logConfig.Level {
	case "debug":
		level = logging.DebugLevel
	case "info":
		level = logging.InfoLevel
	case "warn":
		level = logging.WarnLevel
	case "error":
		level = logging.ErrorLevel
	default:
		level = logging.InfoLevel
	}

	var format logging.LogFormat
	switch logConfig.Format {
	case "json":
		format = logging.JSONFormat
	case "text":
		format = logging.TextFormat
	default:
		format = logging.TextFormat
	}

	var output *os.File
	switch logConfig.Output {
	case "stderr":
		output = os.Stderr
	case "stdout", "":
		output = os.Stdout
	default:
		// For file output, we would open the file here
		output = os.Stdout
	}

	return &logging.LoggerConfig{
		Level:               level,
		Format:              format,
		Output:              output,
		EnableColors:        logConfig.EnableColors,
		EnableTimestamp:     logConfig.EnableTimestamp,
		EnableCaller:        false,
		EnableStackTrace:    logConfig.EnableStackTrace,
		StructuredMetadata:  logConfig.StructuredMetadata,
		MaxFieldLength:      1000,
		TimestampFormat:     "2006-01-02T15:04:05.000Z07:00",
	}
}

func convertToAggregatorConfig(logConfig *config.LoggingConfig) *logging.AggregatorConfig {
	return &logging.AggregatorConfig{
		BufferSize:      1000,
		FlushInterval:   30 * 1000 * 1000 * 1000, // 30 seconds in nanoseconds
		MaxRetries:      3,
		RetryDelay:      1 * 1000 * 1000 * 1000, // 1 second in nanoseconds
		CompressionType: logging.NoCompression,
		EnableRotation:  logConfig.FileRotation,
	}
}

// CreateValidationBuilder creates a validation builder with standard validations
func (ic *InfrastructureContainer) CreateValidationBuilder() *config.ValidationConfigBuilder {
	builder := config.NewValidationConfigBuilder(ic.configManager)
	builder.SetupStandardValidations()
	return builder
}

// ResolveService is a helper method to resolve services from the container
func (ic *InfrastructureContainer) ResolveService(serviceType interface{}) (interface{}, error) {
	return ic.serviceRegistry.ResolveService(serviceType)
}

// MustResolveService resolves a service and panics if it fails (for tests)
func (ic *InfrastructureContainer) MustResolveService(serviceType interface{}) interface{} {
	service, err := ic.ResolveService(serviceType)
	if err != nil {
		panic(fmt.Sprintf("Failed to resolve service %v: %v", reflect.TypeOf(serviceType), err))
	}
	return service
}

// GetInfrastructureInfo returns information about the infrastructure setup
func (ic *InfrastructureContainer) GetInfrastructureInfo() map[string]interface{} {
	return map[string]interface{}{
		"environment":         ic.envConfig.GetEnvironment(),
		"registered_services": len(ic.diContainer.GetRegisteredTypes()),
		"config_values":       len(ic.configManager.GetAllValues()),
		"log_level":          ic.structLogger.GetLevel(),
		"infrastructure_version": "1.0.0",
	}
}