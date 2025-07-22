// Phase R4: ファクトリ登録 - ファクトリパターンベースの複雑なオブジェクト作成
package container

import (
	"fmt"
	"reflect"
	"sync"
)

// FactoryRegistry manages factory patterns for complex object creation
type FactoryRegistry struct {
	factories map[reflect.Type]*FactoryDefinition
	container *DependencyContainer
	mutex     sync.RWMutex
}

// FactoryDefinition holds information about factory registration
type FactoryDefinition struct {
	FactoryType   reflect.Type
	CreationFunc  interface{}
	Dependencies  []reflect.Type
	Configuration map[string]interface{}
	Description   string
}

// NewFactoryRegistry creates a new factory registry
func NewFactoryRegistry(container *DependencyContainer) *FactoryRegistry {
	return &FactoryRegistry{
		factories: make(map[reflect.Type]*FactoryDefinition),
		container: container,
	}
}

// RegisterFactory registers a factory for creating specific types
func (r *FactoryRegistry) RegisterFactory(productType interface{}, factoryType interface{}, creationFunc interface{}) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	pType := reflect.TypeOf(productType)
	if pType.Kind() == reflect.Ptr {
		pType = pType.Elem()
	}

	fType := reflect.TypeOf(factoryType)
	if fType.Kind() == reflect.Ptr {
		fType = fType.Elem()
	}

	// Validate creation function
	creationFuncType := reflect.TypeOf(creationFunc)
	if creationFuncType.Kind() != reflect.Func {
		return fmt.Errorf("creation function must be a function")
	}

	if creationFuncType.NumOut() == 0 {
		return fmt.Errorf("creation function must return at least one value")
	}

	// Extract dependencies from function parameters
	dependencies := make([]reflect.Type, creationFuncType.NumIn())
	for i := 0; i < creationFuncType.NumIn(); i++ {
		dependencies[i] = creationFuncType.In(i)
	}

	r.factories[pType] = &FactoryDefinition{
		FactoryType:  fType,
		CreationFunc: creationFunc,
		Dependencies: dependencies,
	}

	return nil
}

// RegisterFactoryWithConfig registers a factory with configuration
func (r *FactoryRegistry) RegisterFactoryWithConfig(productType interface{}, factoryType interface{}, 
	creationFunc interface{}, config map[string]interface{}, description string) error {
	
	if err := r.RegisterFactory(productType, factoryType, creationFunc); err != nil {
		return err
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	pType := reflect.TypeOf(productType)
	if pType.Kind() == reflect.Ptr {
		pType = pType.Elem()
	}

	if factory, exists := r.factories[pType]; exists {
		factory.Configuration = config
		factory.Description = description
	}

	return nil
}

// CreateInstance creates an instance using registered factory
func (r *FactoryRegistry) CreateInstance(productType interface{}, params map[string]interface{}) (interface{}, error) {
	pType := reflect.TypeOf(productType)
	if pType.Kind() == reflect.Ptr {
		pType = pType.Elem()
	}

	r.mutex.RLock()
	factory, exists := r.factories[pType]
	r.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no factory registered for type %v", pType)
	}

	return r.createInstanceWithFactory(factory, params)
}

// createInstanceWithFactory creates instance using specific factory definition
func (r *FactoryRegistry) createInstanceWithFactory(factory *FactoryDefinition, params map[string]interface{}) (interface{}, error) {
	creationFuncValue := reflect.ValueOf(factory.CreationFunc)
	_ = creationFuncValue.Type() // Suppress unused variable warning

	// Resolve dependencies
	args := make([]reflect.Value, len(factory.Dependencies))
	for i, depType := range factory.Dependencies {
		// Try to resolve from parameters first
		if paramValue, exists := params[depType.String()]; exists {
			args[i] = reflect.ValueOf(paramValue)
			continue
		}

		// Try to resolve from dependency container
		depInstance, err := r.container.Resolve(reflect.New(depType).Elem().Interface())
		if err != nil {
			return nil, fmt.Errorf("failed to resolve dependency %v: %w", depType, err)
		}

		args[i] = reflect.ValueOf(depInstance)
	}

	// Call creation function
	results := creationFuncValue.Call(args)
	if len(results) == 0 {
		return nil, fmt.Errorf("creation function returned no values")
	}

	instance := results[0].Interface()

	// Handle errors returned by creation function
	if len(results) > 1 {
		if err, ok := results[1].Interface().(error); ok && err != nil {
			return nil, fmt.Errorf("creation function returned error: %w", err)
		}
	}

	return instance, nil
}

// GetFactory returns factory definition for a type
func (r *FactoryRegistry) GetFactory(productType interface{}) (*FactoryDefinition, error) {
	pType := reflect.TypeOf(productType)
	if pType.Kind() == reflect.Ptr {
		pType = pType.Elem()
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	factory, exists := r.factories[pType]
	if !exists {
		return nil, fmt.Errorf("no factory registered for type %v", pType)
	}

	return factory, nil
}

// IsFactoryRegistered checks if a factory is registered for a type
func (r *FactoryRegistry) IsFactoryRegistered(productType interface{}) bool {
	pType := reflect.TypeOf(productType)
	if pType.Kind() == reflect.Ptr {
		pType = pType.Elem()
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	_, exists := r.factories[pType]
	return exists
}

// GetRegisteredFactories returns all registered factory types
func (r *FactoryRegistry) GetRegisteredFactories() []reflect.Type {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	types := make([]reflect.Type, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	return types
}

// FactoryBuilder helps build complex factory registrations
type FactoryBuilder struct {
	registry    *FactoryRegistry
	productType interface{}
	factoryType interface{}
	config      map[string]interface{}
	description string
}

// NewFactoryBuilder creates a new factory builder
func (r *FactoryRegistry) NewFactoryBuilder(productType interface{}) *FactoryBuilder {
	return &FactoryBuilder{
		registry:    r,
		productType: productType,
		config:      make(map[string]interface{}),
	}
}

// WithFactoryType sets the factory type
func (b *FactoryBuilder) WithFactoryType(factoryType interface{}) *FactoryBuilder {
	b.factoryType = factoryType
	return b
}

// WithConfig adds configuration
func (b *FactoryBuilder) WithConfig(key string, value interface{}) *FactoryBuilder {
	b.config[key] = value
	return b
}

// WithDescription sets description
func (b *FactoryBuilder) WithDescription(description string) *FactoryBuilder {
	b.description = description
	return b
}

// Register completes the factory registration
func (b *FactoryBuilder) Register(creationFunc interface{}) error {
	return b.registry.RegisterFactoryWithConfig(
		b.productType, 
		b.factoryType, 
		creationFunc, 
		b.config, 
		b.description,
	)
}

// FactoryChain allows chaining multiple factories for complex creation scenarios
type FactoryChain struct {
	registry *FactoryRegistry
	steps    []ChainStep
}

// ChainStep represents a step in factory chain
type ChainStep struct {
	ProductType   reflect.Type
	Configuration map[string]interface{}
	Transform     func(input interface{}) (interface{}, error)
}

// NewFactoryChain creates a new factory chain
func (r *FactoryRegistry) NewFactoryChain() *FactoryChain {
	return &FactoryChain{
		registry: r,
		steps:    make([]ChainStep, 0),
	}
}

// AddStep adds a step to the factory chain
func (c *FactoryChain) AddStep(productType interface{}, config map[string]interface{}, 
	transform func(input interface{}) (interface{}, error)) *FactoryChain {
	
	pType := reflect.TypeOf(productType)
	if pType.Kind() == reflect.Ptr {
		pType = pType.Elem()
	}

	c.steps = append(c.steps, ChainStep{
		ProductType:   pType,
		Configuration: config,
		Transform:     transform,
	})

	return c
}

// Execute executes the factory chain
func (c *FactoryChain) Execute(initialParams map[string]interface{}) (interface{}, error) {
	var result interface{}

	for i, step := range c.steps {
		var input interface{}
		if i == 0 {
			// First step uses initial parameters
			instance, err := c.registry.CreateInstance(
				reflect.New(step.ProductType).Elem().Interface(), 
				initialParams,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create initial instance: %w", err)
			}
			input = instance
		} else {
			input = result
		}

		// Apply transformation if provided
		if step.Transform != nil {
			transformed, err := step.Transform(input)
			if err != nil {
				return nil, fmt.Errorf("transformation failed at step %d: %w", i, err)
			}
			result = transformed
		} else {
			result = input
		}
	}

	return result, nil
}

// GetFactoryInfo returns information about registered factories
func (r *FactoryRegistry) GetFactoryInfo() []FactoryInfo {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	infos := make([]FactoryInfo, 0, len(r.factories))
	for productType, factory := range r.factories {
		infos = append(infos, FactoryInfo{
			ProductType:   productType.String(),
			FactoryType:   factory.FactoryType.String(),
			Dependencies:  typeSliceToStringSlice(factory.Dependencies),
			Configuration: factory.Configuration,
			Description:   factory.Description,
		})
	}

	return infos
}

// FactoryInfo provides information about a registered factory
type FactoryInfo struct {
	ProductType   string                 `json:"product_type"`
	FactoryType   string                 `json:"factory_type"`
	Dependencies  []string               `json:"dependencies"`
	Configuration map[string]interface{} `json:"configuration"`
	Description   string                 `json:"description"`
}

// helper function to convert []reflect.Type to []string
func typeSliceToStringSlice(types []reflect.Type) []string {
	strings := make([]string, len(types))
	for i, t := range types {
		strings[i] = t.String()
	}
	return strings
}