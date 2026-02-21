package prompt

import (
	"testing"
)

func TestRender(t *testing.T) {
	vars := []VariableDef{
		{Name: "company", Type: "string", Required: true},
		{Name: "tone", Type: "enum", Required: false, Default: "professional", Values: []string{"professional", "casual"}},
		{Name: "lang", Type: "string", Required: false, Default: "English"},
	}

	tests := []struct {
		name     string
		tmpl     string
		values   map[string]string
		want     string
		wantErr  bool
	}{
		{
			name:   "all variables provided",
			tmpl:   "Hello from {{company}} in {{lang}} with {{tone}} tone.",
			values: map[string]string{"company": "ACME", "lang": "Korean", "tone": "casual"},
			want:   "Hello from ACME in Korean with casual tone.",
		},
		{
			name:   "defaults applied",
			tmpl:   "{{company}} uses {{tone}} tone in {{lang}}.",
			values: map[string]string{"company": "Corp"},
			want:   "Corp uses professional tone in English.",
		},
		{
			name:    "required variable missing",
			tmpl:    "Hello {{company}}",
			values:  map[string]string{},
			wantErr: true,
		},
		{
			name:   "unknown variable untouched",
			tmpl:   "{{known}} and {{unknown}}",
			values: map[string]string{"known": "A", "company": "X"},
			want:   "A and {{unknown}}",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.tmpl, vars, tc.values)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCountTokens(t *testing.T) {
	tests := []struct {
		text      string
		wantAtLeast int
	}{
		{"Hello world", 1},
		{"You are a helpful assistant.", 4},
		{"", 0},
	}
	for _, tc := range tests {
		got := CountTokens(tc.text)
		if got < tc.wantAtLeast {
			t.Errorf("CountTokens(%q) = %d, want >= %d", tc.text, got, tc.wantAtLeast)
		}
	}
}
