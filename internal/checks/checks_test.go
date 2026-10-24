/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package checks

import (
	"strings"
	"testing"
)

func TestEvaluateValue(t *testing.T) {
	tests := []struct {
		name        string
		val         any
		expect      string
		wantPassed  bool
		wantMessage string
	}{
		{
			name:        "boolean true matching default expectation",
			val:         true,
			expect:      "",
			wantPassed:  true,
			wantMessage: "",
		},
		{
			name:        "postgres byte t matches true",
			val:         []byte("t"),
			expect:      "true",
			wantPassed:  true,
			wantMessage: "",
		},
		{
			name:        "postgres string t matches true",
			val:         "t",
			expect:      "true",
			wantPassed:  true,
			wantMessage: "",
		},
		{
			name:        "postgres byte f matches false",
			val:         []byte("f"),
			expect:      "false",
			wantPassed:  true,
			wantMessage: "",
		},
		{
			name:        "integer matching",
			val:         int64(42),
			expect:      "42",
			wantPassed:  true,
			wantMessage: "",
		},
		{
			name:        "float matching",
			val:         float64(3.14),
			expect:      "3.14",
			wantPassed:  true,
			wantMessage: "",
		},
		{
			name:        "string matching exactly",
			val:         "healthy",
			expect:      "healthy",
			wantPassed:  true,
			wantMessage: "",
		},
		{
			name:        "boolean mismatch echoes actual boolean",
			val:         false,
			expect:      "true",
			wantPassed:  false,
			wantMessage: `expected "true", got "false"`,
		},
		{
			name:        "numeric mismatch echoes actual short number",
			val:         0,
			expect:      "1",
			wantPassed:  false,
			wantMessage: `expected "1", got "0"`,
		},
		{
			name:        "sensitive string mismatch does not leak actual data",
			val:         "super_secret_user_password_hash",
			expect:      "expected_hash",
			wantPassed:  false,
			wantMessage: `expected "expected_hash", got non-matching value`,
		},
		{
			name:        "long number (> 20 chars) does not echo full content",
			val:         "123456789012345678901",
			expect:      "999",
			wantPassed:  false,
			wantMessage: `expected "999", got non-matching value`,
		},
		{
			name:        "nil value",
			val:         nil,
			expect:      "null",
			wantPassed:  true,
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			passed, msg := EvaluateValue(tt.val, tt.expect)
			if passed != tt.wantPassed {
				t.Errorf("EvaluateValue() passed = %v, want %v", passed, tt.wantPassed)
			}
			if msg != tt.wantMessage {
				t.Errorf("EvaluateValue() message = %q, want %q", msg, tt.wantMessage)
			}
		})
	}
}

func TestSanitizeMessage(t *testing.T) {
	msg := strings.Repeat("x", 150)
	sanitized := sanitizeMessage(msg)
	if len(sanitized) > 120 {
		t.Errorf("sanitized length %d exceeds 120", len(sanitized))
	}
	if !strings.HasSuffix(sanitized, "...") {
		t.Errorf("expected ellipsis suffix on truncated message")
	}

	withLines := "hello\nworld\r\ntest"
	sanitizedLines := sanitizeMessage(withLines)
	if strings.Contains(sanitizedLines, "\n") || strings.Contains(sanitizedLines, "\r") {
		t.Errorf("newlines not removed: %q", sanitizedLines)
	}
}
