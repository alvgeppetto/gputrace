package counter

import (
	"testing"
)

func TestRepeatChar(t *testing.T) {
	tests := []struct {
		name     string
		char     byte
		count    int
		expected string
	}{
		{"zero count", '-', 0, ""},
		{"single char", '-', 1, "-"},
		{"multiple chars", '=', 5, "====="},
		{"space char", ' ', 3, "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := repeatChar(tt.char, tt.count)
			if result != tt.expected {
				t.Errorf("repeatChar(%q, %d) = %q, want %q", tt.char, tt.count, result, tt.expected)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"empty string", "", 10, ""},
		{"shorter than max", "hello", 10, "hello"},
		{"exactly max length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 8, "hello..."},
		{"very short max", "hello", 3, "hel"}, // maxLen <= 3 returns truncated without ellipsis
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
			if len(result) > tt.maxLen {
				t.Errorf("truncateString result length %d exceeds maxLen %d", len(result), tt.maxLen)
			}
		})
	}
}
