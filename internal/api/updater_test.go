package api

import (
	"testing"
)

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"v1.0.8", "v1.0.7", 1},
		{"v1.0.7", "v1.0.8", -1},
		{"v1.0.8", "v1.0.8", 0},
		{"1.0.8", "v1.0.8", 0},
		{"v1.1.0", "v1.0.9", 1},
		{"v2.0.0", "v1.99.99", 1},
	}

	for _, tt := range tests {
		got := compareSemver(tt.v1, tt.v2)
		if got != tt.expected {
			t.Errorf("compareSemver(%q, %q) = %d; want %d", tt.v1, tt.v2, got, tt.expected)
		}
	}
}
