package vertexai

import "testing"

func TestNormalizeResponseSchema_ConvertsNullableObjectType(t *testing.T) {
	schema := map[string]any{
		"type": []string{"object", "null"},
		"properties": map[string]any{
			"next_question": map[string]any{"type": "string"},
		},
		"required":             []string{"next_question"},
		"additionalProperties": false,
	}

	got := NormalizeResponseSchema(schema)

	if got["type"] != "OBJECT" {
		t.Fatalf("type = %#v, want OBJECT", got["type"])
	}
	if got["nullable"] != true {
		t.Fatalf("nullable = %#v, want true", got["nullable"])
	}
	if _, ok := got["additionalProperties"]; ok {
		t.Fatalf("additionalProperties should be omitted: %#v", got)
	}
	properties, _ := got["properties"].(map[string]any)
	nextQuestion, _ := properties["next_question"].(map[string]any)
	if nextQuestion["type"] != "STRING" {
		t.Fatalf("next_question.type = %#v, want STRING", nextQuestion["type"])
	}
}
