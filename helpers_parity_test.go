package featurevisor

import "testing"

func TestGetValueByTypeBooleanParity(t *testing.T) {
	if value := GetValueByType(true, "boolean"); value != true {
		t.Fatalf("expected true bool to remain true")
	}
	if value := GetValueByType("true", "boolean"); value != nil {
		t.Fatalf("expected string \"true\" to not be coerced to true")
	}
	if value := GetValueByType(1, "boolean"); value != nil {
		t.Fatalf("expected numeric value to not be coerced to true")
	}
}

func TestGetValueByTypeDoesNotCoerceStrings(t *testing.T) {
	if value := GetValueByType("1", "integer"); value != nil {
		t.Fatalf("expected nil integer, got %#v", value)
	}
	if value := GetValueByType("1.1", "double"); value != nil {
		t.Fatalf("expected nil double, got %#v", value)
	}
	if value := GetValueByType(1.5, "integer"); value != nil {
		t.Fatalf("expected nil fractional integer, got %#v", value)
	}
}
