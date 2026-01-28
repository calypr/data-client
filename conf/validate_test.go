package conf

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func createTestToken(exp time.Time, iat time.Time) string {
	claims := jwt.MapClaims{
		"exp": exp.Unix(),
		"iat": iat.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// We don't need a real signature for ParseUnverified
	tokenString, _ := token.SignedString([]byte("secret"))
	return tokenString
}

func TestIsTokenValid(t *testing.T) {
	man := &Manager{}
	now := time.Now().UTC()

	tests := []struct {
		name    string
		token   string
		want    bool
		wantErr bool
	}{
		{
			name:    "Valid Token",
			token:   createTestToken(now.Add(time.Hour), now.Add(-time.Hour)),
			want:    true,
			wantErr: false,
		},
		{
			name:    "Expired Token",
			token:   createTestToken(now.Add(-time.Hour), now.Add(-2*time.Hour)),
			want:    false,
			wantErr: true,
		},
		{
			name:    "Not Yet Valid Token",
			token:   createTestToken(now.Add(2*time.Hour), now.Add(time.Hour)),
			want:    false,
			wantErr: true,
		},
		{
			name:    "Empty Token",
			token:   "",
			want:    false,
			wantErr: true,
		},
		{
			name:    "Invalid Token Format",
			token:   "not.a.token",
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := man.IsTokenValid(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsTokenValid() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsTokenValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCredentialValid(t *testing.T) {
	man := &Manager{}
	now := time.Now().UTC()
	validToken := createTestToken(now.Add(time.Hour), now.Add(-time.Hour))
	expiredToken := createTestToken(now.Add(-time.Hour), now.Add(-2*time.Hour))

	tests := []struct {
		name    string
		cred    *Credential
		want    bool
		wantErr bool
	}{
		{
			name: "Both Valid",
			cred: &Credential{
				AccessToken: validToken,
				APIKey:      validToken,
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "AccessToken Invalid, APIKey Valid (Needs Refresh)",
			cred: &Credential{
				AccessToken: expiredToken,
				APIKey:      validToken,
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "Both Invalid",
			cred: &Credential{
				AccessToken: expiredToken,
				APIKey:      expiredToken,
			},
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := man.IsCredentialValid(tt.cred)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsCredentialValid() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsCredentialValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
