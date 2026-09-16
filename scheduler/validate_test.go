package main

import "testing"

func TestValidVideoID(t *testing.T) {
	tests := []struct {
		name string
		vid  string
		want bool
	}{
		{"valid uuid", "a3f5c2e1-1234-4abc-8def-1234567890ab", true},
		{"uppercase uuid", "A3F5C2E1-1234-4ABC-8DEF-1234567890AB", true},
		{"empty", "", false},
		{"path traversal", "../.env", false},
		{"deep traversal", "../../etc/passwd", false},
		{"too short", "abc", false},
		{"not uuid", "hello-world", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidVideoID(tt.vid); got != tt.want {
				t.Fatalf("ValidVideoID(%q) = %v, want %v", tt.vid, got, tt.want)
			}
		})
	}
}
