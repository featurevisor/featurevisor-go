package featurevisor

import "testing"

func TestMyType(t *testing.T) {
    // Create a new instance of MyType
    t1 := myType(1)

    // Check that the value of the instance is correct
    if t1 != 1 {
        t.Errorf("Expected 1, but got %d", t1)
    }
}
