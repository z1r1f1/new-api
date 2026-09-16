package openai

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// normalizeResponsesToolSchemas translates legacy nested function definitions
// to the flat Responses format and closes object schemas as required by the
// upstream Responses API. It intentionally operates on raw JSON because the
// protocol also permits provider-specific tool definitions.
func normalizeResponsesToolSchemas(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var tools []map[string]any
	if err := common.Unmarshal(raw, &tools); err != nil {
		return raw
	}
	changed := false
	for index, tool := range tools {
		if toolType, _ := tool["type"].(string); toolType != "function" {
			continue
		}
		tools[index] = normalizeResponsesFunctionTool(tool)
		changed = true
	}
	if !changed {
		return raw
	}
	normalized, err := common.Marshal(tools)
	if err != nil {
		return raw
	}
	return normalized
}

func normalizeResponsesFunctionTool(tool map[string]any) map[string]any {
	if tool == nil {
		return tool
	}
	if function, ok := tool["function"].(map[string]any); ok {
		if name, _ := tool["name"].(string); strings.TrimSpace(name) == "" {
			if name, _ := function["name"].(string); strings.TrimSpace(name) != "" {
				tool["name"] = strings.TrimSpace(name)
			}
		}
		if description, _ := tool["description"].(string); strings.TrimSpace(description) == "" {
			if description, _ := function["description"].(string); strings.TrimSpace(description) != "" {
				tool["description"] = description
			}
		}
		if _, exists := tool["parameters"]; !exists {
			if parameters, exists := function["parameters"]; exists {
				tool["parameters"] = parameters
			}
		}
		delete(tool, "function")
	}
	tool["parameters"] = closeResponsesObjectSchema(tool["parameters"])
	if name, _ := tool["name"].(string); strings.EqualFold(strings.TrimSpace(name), "Read") {
		removeReadPagesParameter(tool["parameters"])
	}
	return tool
}

func closeResponsesObjectSchema(parameters any) any {
	if parameters == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	}
	bytes, err := common.Marshal(parameters)
	if err != nil {
		return closeResponsesObjectSchema(nil)
	}
	var schema map[string]any
	if err := common.Unmarshal(bytes, &schema); err != nil || len(schema) == 0 {
		return closeResponsesObjectSchema(nil)
	}
	return closeResponsesSchemaValue(schema)
}

func closeResponsesSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed)+1)
		for key, child := range typed {
			if child == nil {
				continue
			}
			switch key {
			case "properties":
				if properties, ok := child.(map[string]any); ok {
					closed := make(map[string]any, len(properties))
					for name, property := range properties {
						closed[name] = closeResponsesSchemaValue(property)
					}
					out[key] = closed
					continue
				}
			case "required":
				if _, ok := child.([]any); !ok {
					continue
				}
			case "items":
				out[key] = closeResponsesSchemaValue(child)
				continue
			case "anyOf", "oneOf", "allOf":
				if variants, ok := child.([]any); ok {
					closed := make([]any, 0, len(variants))
					for _, variant := range variants {
						closed = append(closed, closeResponsesSchemaValue(variant))
					}
					out[key] = closed
					continue
				}
			}
			out[key] = child
		}
		if _, hasProperties := out["properties"]; hasProperties {
			if _, hasAdditionalProperties := out["additionalProperties"]; !hasAdditionalProperties {
				out["additionalProperties"] = false
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, closeResponsesSchemaValue(item))
		}
		return out
	default:
		return value
	}
}

func removeReadPagesParameter(parameters any) {
	schema, ok := parameters.(map[string]any)
	if !ok {
		return
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		delete(properties, "pages")
	}
	if required, ok := schema["required"].([]any); ok {
		filtered := make([]any, 0, len(required))
		for _, item := range required {
			if item != "pages" {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) == 0 {
			delete(schema, "required")
		} else {
			schema["required"] = filtered
		}
	}
}
