package featurevisor

import (
	"testing"
	"time"
)

func TestNativeTimestampComparisons(t *testing.T) {
	for _, stamp := range []string{"2026-01-01T00:00:00Z", "1969-12-31T23:59:59.999Z"} {
		boundary, _ := time.Parse(time.RFC3339Nano, stamp)
		for _, offset := range []int{-14 * 3600, 0, 14 * 3600} {
			host := boundary.Add(999999 * time.Nanosecond).In(time.FixedZone("test", offset))
			for _, op := range []Operator{OperatorBefore, OperatorAfter} {
				condition := PlainCondition{Attribute: "date", Operator: op, Value: conditionValue(stamp)}
				if conditionIsMatched(condition, Context{"date": host}, nil) {
					t.Fatalf("%v must equal %s at millisecond precision for %s", host, stamp, op)
				}
				condition.Value = conditionValue(boundary.Add(time.Millisecond).Format(time.RFC3339Nano))
				if got := conditionIsMatched(condition, Context{"date": host}, nil); got != (op == OperatorBefore) {
					t.Fatalf("adjacent millisecond comparison failed for %s", op)
				}
				condition.Value = conditionValue(boundary.Add(-time.Millisecond).Format(time.RFC3339Nano))
				if got := conditionIsMatched(condition, Context{"date": host}, nil); got != (op == OperatorAfter) {
					t.Fatalf("preceding millisecond comparison failed for %s", op)
				}
			}
		}
	}
}

func TestNativeTimestampBoundsAndInvalidOperands(t *testing.T) {
	for _, stamp := range []string{"0001-01-01T00:00:00Z", "9999-12-31T23:59:59.999Z"} {
		host, _ := time.Parse(time.RFC3339Nano, stamp)
		parsed, ok := parsePortableDate(host)
		if !ok || !parsed.Equal(host) {
			t.Fatalf("valid endpoint rejected: %s", stamp)
		}
	}
	invalid := []interface{}{
		time.Date(0, 12, 31, 23, 0, 0, 0, time.FixedZone("negative", -3600)),
		time.Date(10000, 1, 1, 0, 0, 0, 0, time.FixedZone("positive", 3600)),
		time.Date(1, 1, 1, 0, 0, 0, 0, time.FixedZone("positive", 3600)),
		time.Date(9999, 12, 31, 23, 59, 59, 0, time.FixedZone("negative", -3600)),
		"2026-01-01", "2026-01-01T00:00:00", 1767225600000, nil,
	}
	for _, offset := range []int{-50401, 50401, -54000, 54000} {
		invalid = append(invalid, time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("invalid", offset)))
	}
	for _, actual := range invalid {
		for _, op := range []Operator{OperatorBefore, OperatorAfter} {
			condition := PlainCondition{Attribute: "date", Operator: op, Value: conditionValue("2026-01-01T00:00:00Z")}
			if conditionIsMatched(condition, Context{"date": actual}, nil) {
				t.Fatalf("invalid actual matched %s: %v", op, actual)
			}
			condition.Value = conditionValue(actual)
			if conditionIsMatched(condition, Context{"date": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, nil) {
				t.Fatalf("invalid expected matched %s: %v", op, actual)
			}
		}
	}
	if parsed, ok := parsePortableDate(time.Time{}); !ok || parsed.Year() != 1 {
		t.Fatal("zero time is a valid year 0001 instant")
	}
}
