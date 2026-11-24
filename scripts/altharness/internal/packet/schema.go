package packet

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const lintTargetPattern = `^(?!.*\.\.)(?!/)[A-Za-z0-9._/-]+$`

var lintTargetBody = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

func resolveRef(root map[string]any, ref string) (map[string]any, error) {
	const prefix = "#/$defs/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, pErr("unsupported JSON Schema $ref: %s", ref)
	}
	name := ref[len(prefix):]
	defs, ok := asMap(root["$defs"])
	if !ok {
		return nil, pErr("unresolvable JSON Schema $ref: %s", ref)
	}
	node, ok := asMap(defs[name])
	if !ok {
		return nil, pErr("unresolvable JSON Schema $ref: %s", ref)
	}
	return node, nil
}

func matchesSchemaType(value any, expected string) bool {
	switch expected {
	case "integer":
		_, ok := asInt(value)
		return ok
	case "number":
		_, ok := asFloat(value)
		return ok && !isBool(value)
	case "null":
		return value == nil
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		return isBool(value)
	default:
		return false
	}
}

func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case int64, int:
		return "int"
	case float64:
		return "float"
	case string:
		return "str"
	case []any:
		return "list"
	case map[string]any:
		return "dict"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func schemaSearch(pattern, value string) (bool, error) {
	if pattern == lintTargetPattern {
		if strings.Contains(value, "..") || strings.HasPrefix(value, "/") {
			return false, nil
		}
		return lintTargetBody.MatchString(value), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(value), nil
}

func validateStringKeywords(value string, node map[string]any, jsonPath string, issues *[]Issue) {
	if minLen, ok := asInt(node["minLength"]); ok && int64(utf8.RuneCountInString(value)) < minLen {
		*issues = append(*issues, Issue{
			Code:    "SCHEMA_MIN_LENGTH",
			Path:    jsonPath,
			Message: fmt.Sprintf("string length %d is shorter than minimum %d", utf8.RuneCountInString(value), minLen),
		})
	}
	if pattern, ok := asString(node["pattern"]); ok {
		matched, err := schemaSearch(pattern, value)
		if err != nil || !matched {
			*issues = append(*issues, Issue{
				Code:    "SCHEMA_PATTERN",
				Path:    jsonPath,
				Message: fmt.Sprintf("value %q does not match required pattern %q", value, pattern),
			})
		}
	}
}

func validateNumberKeywords(value float64, node map[string]any, jsonPath string, issues *[]Issue) {
	if minimum, ok := asFloat(node["minimum"]); ok && !isBool(node["minimum"]) && value < minimum {
		*issues = append(*issues, Issue{
			Code:    "SCHEMA_MINIMUM",
			Path:    jsonPath,
			Message: fmt.Sprintf("value %v is less than minimum %v", value, minimum),
		})
	}
	if maximum, ok := asFloat(node["maximum"]); ok && !isBool(node["maximum"]) && value > maximum {
		*issues = append(*issues, Issue{
			Code:    "SCHEMA_MAXIMUM",
			Path:    jsonPath,
			Message: fmt.Sprintf("value %v exceeds maximum %v", value, maximum),
		})
	}
}

func validateArrayKeywords(value []any, node, root map[string]any, jsonPath string, issues *[]Issue) error {
	if minItems, ok := asInt(node["minItems"]); ok && int64(len(value)) < minItems {
		*issues = append(*issues, Issue{
			Code:    "SCHEMA_MIN_ITEMS",
			Path:    jsonPath,
			Message: fmt.Sprintf("array has %d item(s); minimum is %d", len(value), minItems),
		})
	}
	if node["uniqueItems"] == true {
		var seen []string
		for idx, item := range value {
			encoded := canonicalJSON(item)
			dup := false
			for _, prev := range seen {
				if prev == encoded {
					dup = true
					break
				}
			}
			if dup {
				*issues = append(*issues, Issue{
					Code:    "SCHEMA_UNIQUE_ITEMS",
					Path:    fmt.Sprintf("%s[%d]", jsonPath, idx),
					Message: fmt.Sprintf("duplicate item %v is not allowed", item),
				})
			} else {
				seen = append(seen, encoded)
			}
		}
	}
	if itemSchema, ok := asMap(node["items"]); ok {
		for idx, item := range value {
			if err := validateSchemaNode(item, itemSchema, root, fmt.Sprintf("%s[%d]", jsonPath, idx), issues); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateObjectKeywords(value, node, root map[string]any, jsonPath string, issues *[]Issue) error {
	if required, ok := asSlice(node["required"]); ok {
		for _, req := range required {
			reqKey, ok := asString(req)
			if !ok {
				continue
			}
			if _, present := value[reqKey]; !present {
				*issues = append(*issues, Issue{
					Code:    "SCHEMA_REQUIRED",
					Path:    jsonPath + "." + reqKey,
					Message: fmt.Sprintf("missing required property '%s'", reqKey),
				})
			}
		}
	}
	props, _ := asMap(node["properties"])
	allowed := map[string]struct{}{}
	for k := range props {
		allowed[k] = struct{}{}
	}
	if node["additionalProperties"] == false {
		for _, key := range sortedKeys(value) {
			if _, ok := allowed[key]; ok {
				continue
			}
			child := jsonPath + "." + key
			if timeFieldRe.MatchString(key) {
				*issues = append(*issues, Issue{
					Code:    "FORBIDDEN_TIME_CONSTRAINT",
					Path:    child,
					Message: fmt.Sprintf("time-based constraint '%s' is forbidden; constrain tasks by work volume (max_files/max_functions), not time", key),
				})
			} else {
				*issues = append(*issues, Issue{
					Code:    "SCHEMA_ADDITIONAL_PROPERTY",
					Path:    child,
					Message: fmt.Sprintf("unexpected property '%s' (additionalProperties: false)", key),
				})
			}
		}
	}
	for key, propSchema := range props {
		if _, present := value[key]; !present {
			continue
		}
		child, ok := asMap(propSchema)
		if !ok {
			continue
		}
		if err := validateSchemaNode(value[key], child, root, jsonPath+"."+key, issues); err != nil {
			return err
		}
	}
	return nil
}

func validateOneOf(value any, branches []any, root map[string]any, jsonPath string, issues *[]Issue) error {
	valid := 0
	var branchErrors [][]Issue
	for _, branch := range branches {
		node, ok := asMap(branch)
		if !ok {
			continue
		}
		var sub []Issue
		if err := validateSchemaNode(value, node, root, jsonPath, &sub); err != nil {
			return err
		}
		if len(sub) == 0 {
			valid++
		} else {
			branchErrors = append(branchErrors, sub)
		}
	}
	if valid != 1 {
		detail := "ambiguous oneOf match"
		if len(branchErrors) > 0 && len(branchErrors[0]) > 0 {
			detail = branchErrors[0][0].Message
		}
		*issues = append(*issues, Issue{
			Code:    "SCHEMA_ONE_OF",
			Path:    jsonPath,
			Message: fmt.Sprintf("value did not match exactly one allowed schema branch (%s)", detail),
		})
	}
	return nil
}

func validateSchemaNode(value any, node, root map[string]any, jsonPath string, issues *[]Issue) error {
	if ref, ok := asString(node["$ref"]); ok {
		target, err := resolveRef(root, ref)
		if err != nil {
			return err
		}
		return validateSchemaNode(value, target, root, jsonPath, issues)
	}
	if branches, ok := asSlice(node["oneOf"]); ok {
		return validateOneOf(value, branches, root, jsonPath, issues)
	}
	if _, ok := node["const"]; ok && !strictEqual(value, node["const"]) {
		*issues = append(*issues, Issue{
			Code:    "SCHEMA_CONST",
			Path:    jsonPath,
			Message: fmt.Sprintf("expected constant value %v, got %v", node["const"], value),
		})
		return nil
	}
	if enum, ok := asSlice(node["enum"]); ok {
		matched := false
		for _, option := range enum {
			if strictEqual(value, option) {
				matched = true
				break
			}
		}
		if !matched {
			*issues = append(*issues, Issue{
				Code:    "SCHEMA_ENUM",
				Path:    jsonPath,
				Message: fmt.Sprintf("value %v must be one of %v", value, enum),
			})
			return nil
		}
	}
	if schemaType, ok := asString(node["type"]); ok && !matchesSchemaType(value, schemaType) {
		*issues = append(*issues, Issue{
			Code:    "SCHEMA_TYPE",
			Path:    jsonPath,
			Message: fmt.Sprintf("expected type '%s', got %s (%v)", schemaType, typeName(value), value),
		})
		return nil
	}
	if notNode, ok := asMap(node["not"]); ok {
		var neg []Issue
		if err := validateSchemaNode(value, notNode, root, jsonPath, &neg); err != nil {
			return err
		}
		if len(neg) == 0 {
			*issues = append(*issues, Issue{
				Code:    "SCHEMA_NOT",
				Path:    jsonPath,
				Message: fmt.Sprintf("value %v matches a forbidden pattern", value),
			})
		}
	}
	switch v := value.(type) {
	case string:
		validateStringKeywords(v, node, jsonPath, issues)
	case int64:
		validateNumberKeywords(float64(v), node, jsonPath, issues)
	case float64:
		validateNumberKeywords(v, node, jsonPath, issues)
	case []any:
		if err := validateArrayKeywords(v, node, root, jsonPath, issues); err != nil {
			return err
		}
	case map[string]any:
		if err := validateObjectKeywords(v, node, root, jsonPath, issues); err != nil {
			return err
		}
	}
	return nil
}
