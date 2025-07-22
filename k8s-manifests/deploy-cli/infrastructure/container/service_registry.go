// Phase R4: サービス登録 - サービスライフサイクル管理
package container

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// ServiceRegistry manages high-level service registration and lifecycle
type ServiceRegistry struct {
	container    *DependencyContainer
	interceptors []ServiceInterceptor
	metrics      *ServiceMetrics
	mutex        sync.RWMutex
}

// ServiceInterceptor allows hooking into service creation
type ServiceInterceptor interface {
	BeforeCreate(serviceType reflect.Type) error
	AfterCreate(serviceType reflect.Type, instance interface{}, duration time.Duration) error
}

// ServiceMetrics tracks service creation metrics
type ServiceMetrics struct {
	CreationCount map[reflect.Type]int64
	CreationTime  map[reflect.Type]time.Duration
	ErrorCount    map[reflect.Type]int64
	mutex         sync.RWMutex
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry(container *DependencyContainer) *ServiceRegistry {
	return &ServiceRegistry{
		container: container,
		metrics: &ServiceMetrics{
			CreationCount: make(map[reflect.Type]int64),
			CreationTime:  make(map[reflect.Type]time.Duration),
			ErrorCount:    make(map[reflect.Type]int64),
		},
	}
}

// AddInterceptor adds a service interceptor
func (r *ServiceRegistry) AddInterceptor(interceptor ServiceInterceptor) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.interceptors = append(r.interceptors, interceptor)
}

// RegisterService registers a service with advanced options
func (r *ServiceRegistry) RegisterService(options *ServiceRegistrationOptions) error {
	if options.ServiceType == nil {
		return fmt.Errorf("ServiceType is required")
	}
	
	if options.Factory == nil && options.Instance == nil {
		return fmt.Errorf("either Factory or Instance must be provided")
	}

	// Validate dependencies if specified
	if len(options.Dependencies) > 0 {
		if err := r.validateDependencies(options.Dependencies); err != nil {
			return fmt.Errorf("dependency validation failed: %w", err)
		}
	}

	// Register with container
	if options.Instance != nil {
		return r.container.RegisterInstance(options.ServiceType, options.Instance)
	}

	if options.Lifecycle == Singleton {
		return r.container.RegisterSingleton(options.ServiceType, options.Factory)
	}

	return r.container.RegisterTransient(options.ServiceType, options.Factory)
}

// ResolveService resolves a service with metrics and interceptors
func (r *ServiceRegistry) ResolveService(serviceType interface{}) (interface{}, error) {
	t := reflect.TypeOf(serviceType)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Record metrics
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		r.recordMetrics(t, duration, nil)
	}()

	// Execute before interceptors
	for _, interceptor := range r.interceptors {
		if err := interceptor.BeforeCreate(t); err != nil {
			r.recordError(t)
			return nil, fmt.Errorf("before interceptor failed: %w", err)
		}
	}

	// Resolve service
	instance, err := r.container.Resolve(serviceType)
	if err != nil {
		r.recordError(t)
		return nil, err
	}

	// Execute after interceptors
	duration := time.Since(start)
	for _, interceptor := range r.interceptors {
		if err := interceptor.AfterCreate(t, instance, duration); err != nil {
			r.recordError(t)
			return nil, fmt.Errorf("after interceptor failed: %w", err)
		}
	}

	return instance, nil
}

// ServiceRegistrationOptions provides options for service registration
type ServiceRegistrationOptions struct {
	ServiceType  interface{}
	Factory      interface{}
	Instance     interface{}
	Lifecycle    ServiceLifecycle
	Dependencies []interface{}
	Tags         map[string]string
	Description  string
}

// validateDependencies validates that all dependencies are registered
func (r *ServiceRegistry) validateDependencies(dependencies []interface{}) error {
	for _, dep := range dependencies {
		if !r.container.IsRegistered(dep) {
			return fmt.Errorf("dependency %T is not registered", dep)
		}
	}
	return nil
}

// recordMetrics records service creation metrics
func (r *ServiceRegistry) recordMetrics(serviceType reflect.Type, duration time.Duration, err error) {
	r.metrics.mutex.Lock()
	defer r.metrics.mutex.Unlock()

	if err != nil {
		r.metrics.ErrorCount[serviceType]++
	} else {
		r.metrics.CreationCount[serviceType]++
		
		// Update average creation time
		current := r.metrics.CreationTime[serviceType]
		count := r.metrics.CreationCount[serviceType]
		r.metrics.CreationTime[serviceType] = (current*time.Duration(count-1) + duration) / time.Duration(count)
	}
}

// recordError records service creation error
func (r *ServiceRegistry) recordError(serviceType reflect.Type) {
	r.metrics.mutex.Lock()
	defer r.metrics.mutex.Unlock()
	
	r.metrics.ErrorCount[serviceType]++
}

// GetMetrics returns service creation metrics
func (r *ServiceRegistry) GetMetrics() ServiceMetricsSnapshot {
	r.metrics.mutex.RLock()
	defer r.metrics.mutex.RUnlock()

	snapshot := ServiceMetricsSnapshot{
		CreationCount: make(map[string]int64),
		CreationTime:  make(map[string]string),
		ErrorCount:    make(map[string]int64),
	}

	for t, count := range r.metrics.CreationCount {
		snapshot.CreationCount[t.String()] = count
	}

	for t, duration := range r.metrics.CreationTime {
		snapshot.CreationTime[t.String()] = duration.String()
	}

	for t, count := range r.metrics.ErrorCount {
		snapshot.ErrorCount[t.String()] = count
	}

	return snapshot
}

// ServiceMetricsSnapshot provides a serializable snapshot of metrics
type ServiceMetricsSnapshot struct {
	CreationCount map[string]int64  `json:"creation_count"`
	CreationTime  map[string]string `json:"creation_time"`
	ErrorCount    map[string]int64  `json:"error_count"`
}

// HealthCheck verifies that all registered services can be resolved
func (r *ServiceRegistry) HealthCheck(ctx context.Context) error {
	types := r.container.GetRegisteredTypes()
	
	for _, t := range types {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Try to resolve each service
		instance := reflect.New(t).Elem().Interface()
		_, err := r.container.Resolve(instance)
		if err != nil {
			return fmt.Errorf("failed to resolve service %v during health check: %w", t, err)
		}
	}

	return nil
}

// GetServiceDependencyGraph returns dependency relationships
func (r *ServiceRegistry) GetServiceDependencyGraph() map[string][]string {
	types := r.container.GetRegisteredTypes()
	graph := make(map[string][]string)

	for _, t := range types {
		info, err := r.container.GetServiceInfo(reflect.New(t).Elem().Interface())
		if err != nil {
			continue
		}

		typeName := info.Type.String()
		graph[typeName] = []string{} // Initialize empty dependencies list
		
		// Note: In a more advanced implementation, we would track actual dependencies
		// from factory function parameters
	}

	return graph
}

// Dispose cleans up resources (useful for graceful shutdown)
func (r *ServiceRegistry) Dispose() error {
	// In a more advanced implementation, this would:
	// 1. Call Dispose() on services that implement IDisposable
	// 2. Clear singleton instances
	// 3. Clean up resources

	r.container.Clear()
	return nil
}

// LoggingInterceptor logs service creation events
type LoggingInterceptor struct {
	LogFunc func(level string, message string, args ...interface{})
}

// BeforeCreate logs before service creation
func (l *LoggingInterceptor) BeforeCreate(serviceType reflect.Type) error {
	if l.LogFunc != nil {
		l.LogFunc("debug", "Creating service", "type", serviceType.String())
	}
	return nil
}

// AfterCreate logs after service creation
func (l *LoggingInterceptor) AfterCreate(serviceType reflect.Type, instance interface{}, duration time.Duration) error {
	if l.LogFunc != nil {
		l.LogFunc("debug", "Service created successfully", 
			"type", serviceType.String(), 
			"duration", duration.String())
	}
	return nil
}

// PerformanceInterceptor tracks slow service creations
type PerformanceInterceptor struct {
	SlowThreshold time.Duration
	OnSlowCreation func(serviceType reflect.Type, duration time.Duration)
}

// BeforeCreate implements ServiceInterceptor
func (p *PerformanceInterceptor) BeforeCreate(serviceType reflect.Type) error {
	return nil
}

// AfterCreate tracks performance and reports slow creations
func (p *PerformanceInterceptor) AfterCreate(serviceType reflect.Type, instance interface{}, duration time.Duration) error {
	if p.SlowThreshold > 0 && duration > p.SlowThreshold {
		if p.OnSlowCreation != nil {
			p.OnSlowCreation(serviceType, duration)
		}
	}
	return nil
}