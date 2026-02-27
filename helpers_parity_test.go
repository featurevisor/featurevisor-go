package featurevisor

import "testing"

func TestGetValueByTypeBooleanParity(t *testing.T) {
	if value := GetValueByType(true, "boolean"); value != true {
		t.Fatalf("expected true bool to remain true")
	}
	if value := GetValueByType("true", "boolean"); value != false {
		t.Fatalf("expected string \"true\" to not be coerced to true")
	}
	if value := GetValueByType(1, "boolean"); value != false {
		t.Fatalf("expected numeric value to not be coerced to true")
	}
}
