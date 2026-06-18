package utils

import "testing"

func TestNormalizeSearchText(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "removes diacritics",
			value: "  Crème Brûlée  ",
			want:  "creme brulee",
		},
		{
			name:  "normalizes ampersand connector",
			value: "King Gizzard & The Lizard Wizard",
			want:  "king gizzard and the lizard wizard",
		},
		{
			name:  "preserves word connector",
			value: "King Gizzard and The Lizard Wizard",
			want:  "king gizzard and the lizard wizard",
		},
		{
			name:  "normalizes ampersand without surrounding spaces",
			value: "A&B",
			want:  "a and b",
		},
		{
			name:  "strips punctuation and compresses spaces",
			value: "Cherry-coloured Funk",
			want:  "cherry coloured funk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeSearchText(tt.value)
			if err != nil {
				t.Fatalf("NormalizeSearchText() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("NormalizeSearchText() = %q, want %q", got, tt.want)
			}
		})
	}
}
