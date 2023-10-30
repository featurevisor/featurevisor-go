package featurevisor

import "testing"

func TestType(t *testing.T) {
	// Create a new instance of MyType
	t1 := AttributeKey("userId")

	// Check that the value of the instance is correct
	if t1 != "userId" {
		t.Errorf("Expected userId, but got %s", t1)
	}
}
