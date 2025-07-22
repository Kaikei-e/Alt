// Phase R4: 依存注入コンテナ - 中央集権的な依存関係管理
package container

import (
	"fmt"
	"reflect"
	"sync"
)

// ServiceLifecycle defines the lifecycle of services
type ServiceLifecycle int

const (
	// Singleton creates only one instance throughout the application lifecycle
	Singleton ServiceLifecycle = iota
	// Transient creates a new instance every time it's requested
	Transient
)

// ServiceDefinition holds information about how to create a service
type ServiceDefinition struct {
	Type      reflect.Type
	Factory   interface{}
	Lifecycle ServiceLifecycle
	Instance  interface{}
	mutex     sync.RWMutex
}

// DependencyContainer manages service dependencies with type safety
type DependencyContainer struct {
	services map[reflect.Type]*ServiceDefinition
	mutex    sync.RWMutex
}

// NewDependencyContainer creates a new dependency injection container
func NewDependencyContainer() *DependencyContainer {
	return &DependencyContainer{
		services: make(map[reflect.Type]*ServiceDefinition),
	}
}

// RegisterSingleton registers a service as singleton with factory function
func (c *DependencyContainer) RegisterSingleton(serviceType interface{}, factory interface{}) error {
	return c.register(serviceType, factory, Singleton)
}

// RegisterTransient registers a service as transient with factory function
func (c *DependencyContainer) RegisterTransient(serviceType interface{}, factory interface{}) error {
	return c.register(serviceType, factory, Transient)
}

// RegisterInstance registers an existing instance as singleton
func (c *DependencyContainer) RegisterInstance(serviceType interface{}, instance interface{}) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	t := reflect.TypeOf(serviceType)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Validate instance type compatibility
	instanceType := reflect.TypeOf(instance)
	if !instanceType.AssignableTo(reflect.PtrTo(t)) && !instanceType.AssignableTo(t) {
		return fmt.Errorf("instance type %v is not assignable to service type %v", instanceType, t)
	}

	c.services[t] = &ServiceDefinition{
		Type:      t,
		Factory:   nil,
		Lifecycle: Singleton,
		Instance:  instance,
	}

	return nil
}

// register is the common registration logic
func (c *DependencyContainer) register(serviceType interface{}, factory interface{}, lifecycle ServiceLifecycle) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	t := reflect.TypeOf(serviceType)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Validate factory function
	factoryType := reflect.TypeOf(factory)
	if factoryType.Kind() != reflect.Func {
		return fmt.Errorf("factory must be a function")
	}

	if factoryType.NumOut() == 0 {
		return fmt.Errorf("factory function must return at least one value")
	}

	// Check if return type is compatible
	returnType := factoryType.Out(0)
	if !returnType.AssignableTo(reflect.PtrTo(t)) && !returnType.AssignableTo(t) {
		return fmt.Errorf("factory return type %v is not assignable to service type %v", returnType, t)
	}

	c.services[t] = &ServiceDefinition{
		Type:      t,
		Factory:   factory,
		Lifecycle: lifecycle,
	}

	return nil
}

// Resolve resolves a service by type with dependency injection
func (c *DependencyContainer) Resolve(serviceType interface{}) (interface{}, error) {
	t := reflect.TypeOf(serviceType)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	c.mutex.RLock()
	definition, exists := c.services[t]
	c.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("service of type %v is not registered", t)
	}

	return c.createInstance(definition)
}

// createInstance creates an instance based on service definition
func (c *DependencyContainer) createInstance(definition *ServiceDefinition) (interface{}, error) {
	// For Singleton services, check if instance already exists
	if definition.Lifecycle == Singleton {
		definition.mutex.RLock()
		if definition.Instance != nil {
			instance := definition.Instance
			definition.mutex.RUnlock()
			return instance, nil
		}
		definition.mutex.RUnlock()

		definition.mutex.Lock()
		defer definition.mutex.Unlock()

		// Double-check pattern
		if definition.Instance != nil {
			return definition.Instance, nil
		}
	}

	// Create new instance using factory
	if definition.Factory == nil {
		return nil, fmt.Errorf("no factory function defined for type %v", definition.Type)
	}

	factoryValue := reflect.ValueOf(definition.Factory)
	factoryType := factoryValue.Type()

	// Prepare arguments for factory function
	args := make([]reflect.Value, factoryType.NumIn())
	for i := 0; i < factoryType.NumIn(); i++ {
		argType := factoryType.In(i)
		
		// Resolve dependency recursively
		argInstance, err := c.Resolve(reflect.New(argType).Elem().Interface())
		if err != nil {
			return nil, fmt.Errorf("failed to resolve dependency %v for %v: %w", argType, definition.Type, err)
		}
		
		args[i] = reflect.ValueOf(argInstance)
	}

	// Call factory function
	results := factoryValue.Call(args)
	if len(results) == 0 {
		return nil, fmt.Errorf("factory function returned no values")
	}

	instance := results[0].Interface()

	// Handle errors returned by factory
	if len(results) > 1 {
		if err, ok := results[1].Interface().(error); ok && err != nil {
			return nil, fmt.Errorf("factory function returned error: %w", err)
		}
	}

	// Store instance for Singleton services
	if definition.Lifecycle == Singleton {
		definition.Instance = instance
	}

	return instance, nil
}

// IsRegistered checks if a service type is registered
func (c *DependencyContainer) IsRegistered(serviceType interface{}) bool {
	t := reflect.TypeOf(serviceType)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	_, exists := c.services[t]
	return exists
}

// GetRegisteredTypes returns all registered service types
func (c *DependencyContainer) GetRegisteredTypes() []reflect.Type {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	types := make([]reflect.Type, 0, len(c.services))
	for t := range c.services {
		types = append(types, t)
	}
	return types
}

// Clear removes all registered services (useful for testing)
func (c *DependencyContainer) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.services = make(map[reflect.Type]*ServiceDefinition)
}

// GetServiceInfo returns information about a registered service
func (c *DependencyContainer) GetServiceInfo(serviceType interface{}) (*ServiceInfo, error) {
	t := reflect.TypeOf(serviceType)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	definition, exists := c.services[t]
	if !exists {
		return nil, fmt.Errorf("service of type %v is not registered", t)
	}

	return &ServiceInfo{
		Type:      definition.Type,
		Lifecycle: definition.Lifecycle,
		HasInstance: definition.Instance != nil,
		HasFactory:  definition.Factory != nil,
	}, nil
}

// ServiceInfo provides information about a registered service
type ServiceInfo struct {
	Type        reflect.Type
	Lifecycle   ServiceLifecycle
	HasInstance bool
	HasFactory  bool
}

// String returns string representation of ServiceInfo
func (si *ServiceInfo) String() string {
	lifecycle := "Transient"
	if si.Lifecycle == Singleton {
		lifecycle = "Singleton"
	}
	
	return fmt.Sprintf("Service{Type: %v, Lifecycle: %s, HasInstance: %v, HasFactory: %v}",
		si.Type, lifecycle, si.HasInstance, si.HasFactory)
}