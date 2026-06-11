package githubutil

import "testing"

func TestNormalizeRepositoryURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://github.com/calypr/gecko", "github.com/calypr/gecko"},
		{"https://github.com/calypr/gecko.git", "github.com/calypr/gecko"},
		{"git@github.com:calypr/gecko.git", "github.com/calypr/gecko"},
		{"github.com/calypr/gecko", "github.com/calypr/gecko"},
		{"ssh://git@ssh.github.com:443/calypr/gecko.git", "github.com/calypr/gecko"},
		{"source.ohsu.edu/calypr/gecko", "source.ohsu.edu/calypr/gecko"},
	}

	for _, tt := range tests {
		got, err := NormalizeRepositoryURL(tt.in)
		if err != nil {
			t.Fatalf("NormalizeRepositoryURL(%q) returned error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("NormalizeRepositoryURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeRepositoryURLRejectsInvalidPaths(t *testing.T) {
	bad := []string{
		"",
		"https://github.com/calypr",
		"github.com/calypr",
		"https://github.com/calypr/gecko/extra",
	}

	for _, input := range bad {
		if _, err := NormalizeRepositoryURL(input); err == nil {
			t.Fatalf("expected NormalizeRepositoryURL(%q) to fail", input)
		}
	}
}
