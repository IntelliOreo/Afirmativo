package vertexai

import "strings"

// NormalizeResponseSchema converts the repo's JSON-schema-like maps into the
// subset of the Vertex responseSchema dialect used by generateContent.
func NormalizeResponseSchema(schema map[string]any) map[string]any {
	normalized, _ := normalizeSchemaValue(schema).(map[string]any)
	return normalized
}

func normalizeSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeSchemaMap(typed)
	case []string:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items
	case []any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, normalizeSchemaValue(item))
		}
		return items
	default:
		return value
	}
}

func normalizeSchemaMap(schema map[string]any) map[string]any {
	normalized := make(map[string]any, len(schema))

	nullable := false
	if rawType, ok := schema["type"]; ok {
		switch typed := rawType.(type) {
		case string:
			normalized["type"] = normalizeSchemaType(typed)
		case []string:
			schemaType, collapsedNullable := collapseSchemaTypes(typed)
			if schemaType != "" {
				normalized["type"] = schemaType
			}
			nullable = nullable || collapsedNullable
		case []any:
			stringTypes := make([]string, 0, len(typed))
			for _, item := range typed {
				if text, ok := item.(string); ok {
					stringTypes = append(stringTypes, text)
				}
			}
			schemaType, collapsedNullable := collapseSchemaTypes(stringTypes)
			if schemaType != "" {
				normalized["type"] = schemaType
			}
			nullable = nullable || collapsedNullable
		}
	}

	for key, rawValue := range schema {
		switch key {
		case "type":
			continue
		case "additionalProperties":
			// Vertex responseSchema does not need this field for our use cases.
			continue
		case "properties":
			properties := make(map[string]any)
			switch typed := rawValue.(type) {
			case map[string]any:
				for name, child := range typed {
					properties[name] = normalizeSchemaValue(child)
				}
			}
			normalized["properties"] = properties
		default:
			normalized[key] = normalizeSchemaValue(rawValue)
		}
	}

	if nullable {
		normalized["nullable"] = true
	}
	return normalized
}

func collapseSchemaTypes(types []string) (schemaType string, nullable bool) {
	for _, value := range types {
		if strings.EqualFold(strings.TrimSpace(value), "null") {
			nullable = true
			continue
		}
		if schemaType == "" {
			schemaType = normalizeSchemaType(value)
		}
	}
	return schemaType, nullable
}

func normalizeSchemaType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "object":
		return "OBJECT"
	case "array":
		return "ARRAY"
	case "string":
		return "STRING"
	case "integer":
		return "INTEGER"
	case "number":
		return "NUMBER"
	case "boolean":
		return "BOOLEAN"
	case "null":
		return "NULL"
	default:
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}
