package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"
)

// loadFromEnvironment loads configuration from environment variables
// using reflection to parse struct tags
func loadFromEnvironment(config *Config) error {
	return loadStruct(reflect.ValueOf(config).Elem())
}

func loadStruct(v reflect.Value) error {
	t := v.Type()
	
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		
		// Skip unexported fields
		if !field.CanSet() {
			continue
		}
		
		// Handle nested structs recursively
		if field.Kind() == reflect.Struct && fieldType.Type.Name() != "Duration" {
			if err := loadStruct(field); err != nil {
				return err
			}
			continue
		}
		
		// Get environment variable name and default value from tags
		envTag := fieldType.Tag.Get("env")
		defaultTag := fieldType.Tag.Get("default")
		
		if envTag == "" {
			continue
		}
		
		// Get value from environment, use default if not set
		value := os.Getenv(envTag)
		if value == "" {
			value = defaultTag
		}
		
		// Set the field value based on its type
		if err := setFieldValue(field, value, envTag); err != nil {
			return fmt.Errorf("failed to set field %s: %w", fieldType.Name, err)
		}
	}
	
	return nil
}

func setFieldValue(field reflect.Value, value, envName string) error {
	if value == "" {
		return nil
	}
	
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
		
	case reflect.Int:
		intVal, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value for %s: %s", envName, value)
		}
		field.SetInt(int64(intVal))
		
	case reflect.Int64:
		// Handle time.Duration specially
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("invalid duration value for %s: %s", envName, value)
			}
			field.SetInt(int64(duration))
		} else {
			intVal, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid integer value for %s: %s", envName, value)
			}
			field.SetInt(intVal)
		}
		
	default:
		return fmt.Errorf("unsupported field type %s for %s", field.Kind(), envName)
	}
	
	return nil
}