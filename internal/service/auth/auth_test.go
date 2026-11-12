package auth

import (
	"testing"
	"time"
)

func TestGenerateJWTUsesTTL(t *testing.T) {
	for _, ttl := range []time.Duration{24 * time.Hour, time.Hour} {
		t.Run(ttl.String(), func(t *testing.T) {
			token, err := GenerateJWT("user-id", "user@example.com", "reader", "secret", ttl)
			if err != nil {
				t.Fatalf("GenerateJWT() error = %v", err)
			}

			claims, err := ValidateJWT(token, "secret")
			if err != nil {
				t.Fatalf("ValidateJWT() error = %v", err)
			}
			got := time.Duration(claims.ExpiresAt.Unix()-claims.IssuedAt.Unix()) * time.Second
			if got != ttl {
				t.Errorf("token lifetime = %v, want %v", got, ttl)
			}
		})
	}
}
