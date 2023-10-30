package featurevisor

import (
	"testing"
)

func TestGetBucketedNumber(t *testing.T) {
	if got := getBucketedNumber("foo"); got < 0 || got > MAX_BUCKETED_NUMBER {
		t.Errorf("getBucketedNumber(\"foo\") = %d; want a number between 0 and %d", got, MAX_BUCKETED_NUMBER)
	}
	if got := getBucketedNumber("bar"); got < 0 || got > MAX_BUCKETED_NUMBER {
		t.Errorf("getBucketedNumber(\"bar\") = %d; want a number between 0 and %d", got, MAX_BUCKETED_NUMBER)
	}
	if got := getBucketedNumber("baz"); got < 0 || got > MAX_BUCKETED_NUMBER {
		t.Errorf("getBucketedNumber(\"baz\") = %d; want a number between 0 and %d", got, MAX_BUCKETED_NUMBER)
	}
	if got := getBucketedNumber("123adshlk348-93asdlk"); got < 0 || got > MAX_BUCKETED_NUMBER {
		t.Errorf("getBucketedNumber(\"123adshlk348-93asdlk\") = %d; want a number between 0 and %d", got, MAX_BUCKETED_NUMBER)
	}

	expectedResults := map[string]int{
		"foo":         20602,
		"bar":         89144,
		"123.foo":     3151,
		"123.bar":     9710,
		"123.456.foo": 14432,
		"123.456.bar": 1982,
	}

	for key, expected := range expectedResults {
		if got := getBucketedNumber(key); got != expected {
			t.Errorf("getBucketedNumber(%q) = %d; want %d", key, got, expected)
		}
	}
}
