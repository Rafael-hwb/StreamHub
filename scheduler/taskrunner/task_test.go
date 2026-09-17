package taskrunner

import (
	"context"
	"testing"
)

func TestValidVideoID(t *testing.T) {
	tests := []struct {
		name string
		vid  string
		want bool
	}{
		{"valid uuid", "a3f5c2e1-1234-4abc-8def-1234567890ab", true},
		{"empty", "", false},
		{"path traversal", "../.env", false},
		{"not uuid", "hello-world", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validVideoID(tt.vid); got != tt.want {
				t.Fatalf("validVideoID(%q) = %v, want %v", tt.vid, got, tt.want)
			}
		})
	}
}

func TestDeleteOneRejectsInvalidID(t *testing.T) {
	// 校验先于 store 调用，所以 nil store 是安全的。
	if err := deleteOne(context.Background(), nil, "../etc/passwd"); err == nil {
		t.Fatal("expected error for invalid video id")
	}
}
