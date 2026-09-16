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

func TestSafePath(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		file    string
		wantErr bool
	}{
		{"ok", "./videos", "abc.mp4", false},
		{"traversal", "./videos", "../.env", true},
		{"deep traversal", "./videos", "../../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafePath(tt.dir, tt.file)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SafePath(%q, %q) err = %v, wantErr %v", tt.dir, tt.file, err, tt.wantErr)
			}
		})
	}
}
