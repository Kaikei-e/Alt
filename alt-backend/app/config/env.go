package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
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

	case reflect.Bool:
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean value for %s: %s", envName, value)
		}
		field.SetBool(boolVal)

	case reflect.Int:
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer value for %s: %s", envName, value)
		}
		// Check bounds for int type (platform-dependent) - allow reasonable ranges
		// For 64-bit systems, this allows values up to 2^63-1
		// For 32-bit systems, this allows values up to 2^31-1
		const maxInt64 = 1<<63 - 1
		const minInt64 = -1 << 63
		if intVal > maxInt64 || intVal < minInt64 {
			return fmt.Errorf("integer value out of range for %s: %s (max: %d, min: %d)", envName, value, maxInt64, minInt64)
		}
		field.SetInt(intVal)

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

	case reflect.Slice:
		// Handle []string type
		if field.Type().Elem().Kind() == reflect.String {
			// Split by comma and trim whitespace
			strSlice := strings.Split(value, ",")
			for i, s := range strSlice {
				strSlice[i] = strings.TrimSpace(s)
			}
			field.Set(reflect.ValueOf(strSlice))
		} else {
			return fmt.Errorf("unsupported slice type for %s", envName)
		}

	default:
		return fmt.Errorf("unsupported field type %s for %s", field.Kind(), envName)
	}

	return nil
}
