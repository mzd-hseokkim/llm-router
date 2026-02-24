package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AdminClaims holds admin-specific JWT claims.
type AdminClaims struct {
	jwt.RegisteredClaims
	Username        string `json:"username"`
	PasswordChanged bool   `json:"password_changed"`
}

// JWTService issues and validates admin JWTs.
type JWTService struct {
	signingKey []byte
	expiry     time.Duration
}

// NewJWTService creates a JWTService with the given signing key and token expiry.
func NewJWTService(signingKey []byte, expiry time.Duration) *JWTService {
	return &JWTService{signingKey: signingKey, expiry: expiry}
}

// GenerateToken creates a signed HS256 JWT for the given admin user.
func (s *JWTService) GenerateToken(username string, passwordChanged bool) (string, error) {
	now := time.Now()
	claims := AdminClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expiry)),
		},
		Username:        username,
		PasswordChanged: passwordChanged,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.signingKey)
	if err != nil {
		return "", fmt.Errorf("jwt.GenerateToken: %w", err)
	}
	return signed, nil
}

// ValidateToken parses and validates a JWT, returning the embedded claims.
func (s *JWTService) ValidateToken(tokenString string) (*AdminClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AdminClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.signingKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("jwt.ValidateToken: %w", err)
	}
	claims, ok := token.Claims.(*AdminClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("jwt.ValidateToken: invalid token")
	}
	return claims, nil
}
