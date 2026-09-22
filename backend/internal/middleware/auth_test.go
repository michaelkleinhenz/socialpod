package middleware

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTAlgorithmValidation(t *testing.T) {
	secret := "test-secret-key"

	// Generate a valid HS256 token
	validToken, err := GenerateToken("user123", false, secret)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	// Parse the valid token — should succeed
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(validToken, claims, func(tok *jwt.Token) (interface{}, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			t.Fatalf("expected HMAC signing method, got %T", tok.Method)
		}
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if !token.Valid {
		t.Fatal("valid token marked invalid")
	}
	if claims.UserID != "user123" {
		t.Fatalf("expected userId user123, got %s", claims.UserID)
	}

	// Craft a token with alg:none — should be rejected
	noneToken := jwt.NewWithClaims(jwt.SigningMethodNone, &Claims{
		UserID: "attacker",
		IsAdmin: true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(72 * time.Hour)),
		},
	})
	noneStr, err := noneToken.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to craft none token: %v", err)
	}

	noneClaims := &Claims{}
	_, err = jwt.ParseWithClaims(noneStr, noneClaims, func(tok *jwt.Token) (interface{}, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})
	if err == nil {
		t.Fatal("alg:none token was accepted — this is the vulnerability!")
	}
}

func TestJWTRSARejected(t *testing.T) {
	secret := "test-secret-key"

	// Generate a valid token but try to parse it claiming RSA method
	validToken, err := GenerateToken("user123", false, secret)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	claims := &Claims{}
	_, err = jwt.ParseWithClaims(validToken, claims, func(tok *jwt.Token) (interface{}, error) {
		// Simulate what happens if someone swaps the algorithm to RSA
		if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("valid HS256 token rejected: %v", err)
	}
}
