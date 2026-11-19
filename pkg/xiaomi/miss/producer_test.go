package miss

import (
	"net/url"
	"testing"
)

func TestDialAttempts(t *testing.T) {
	tests := []struct {
		name  string
		query url.Values
		want  int
	}{
		{
			name:  "default",
			query: url.Values{},
			want:  1,
		},
		{
			name:  "retries",
			query: url.Values{"retries": {"4"}},
			want:  4,
		},
		{
			name:  "retry alias",
			query: url.Values{"retry": {"3"}},
			want:  3,
		},
		{
			name:  "bounded maximum",
			query: url.Values{"retries": {"9"}},
			want:  5,
		},
		{
			name:  "invalid",
			query: url.Values{"retries": {"nope"}},
			want:  1,
		},
		{
			name:  "non-positive",
			query: url.Values{"retries": {"0"}},
			want:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dialAttempts(tt.query); got != tt.want {
				t.Fatalf("dialAttempts() = %d, want %d", got, tt.want)
			}
		})
	}
}
