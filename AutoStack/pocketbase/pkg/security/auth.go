// Package security implements authentication, RBAC, rate limiting,
// audit trail, and secret management for the AutoStack platform.
// Credentials are NEVER logged at any log level.
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// TokenType classifies the bearer token kind.
type TokenType string

const (
	TokenTypeJWT     TokenType = "jwt"
	TokenTypeAPI     TokenType = "api"
	TokenTypeService TokenType = "service"
)

// TokenClaims is the verified payload of a validated token.
// The raw token is never stored or returned.
type TokenClaims struct {
	TokenID    string    `json:"token_id"`
	TenantID   string    `json:"tenant_id"`
	Role       Role      `json:"role"`
	TokenType  TokenType `json:"token_type"`
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// AuthResult is the outcome of a token verification. Raw token is never included.
type AuthResult struct {
	Valid   bool
	Claims  TokenClaims
	Reason  string // non-empty only when !Valid
}

// VerifyJWT validates a signed JWT. Returns AuthResult — never returns the raw token.
// Uses HMAC-SHA256 with the provided secret. Deterministic failure on invalid input.
func VerifyJWT(tokenStr, secret, expectedTenantID string) AuthResult {
	if tokenStr == "" || secret == "" {
		return AuthResult{Valid: false, Reason: "empty token or secret"}
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	token, err := parser.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		// Never include err details that might leak token content.
		if errors.Is(err, jwt.ErrTokenExpired) {
			return AuthResult{Valid: false, Reason: "token expired"}
		}
		return AuthResult{Valid: false, Reason: "token validation failed"}
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return AuthResult{Valid: false, Reason: "invalid claims structure"}
	}

	tenantID, _ := claims["tenant_id"].(string)
	if expectedTenantID != "" && tenantID != expectedTenantID {
		return AuthResult{Valid: false, Reason: "tenant mismatch"}
	}

	tokenID, _ := claims["token_id"].(string)
	roleStr, _ := claims["role"].(string)
	tokenTypeStr, _ := claims["token_type"].(string)

	var issuedAt, expiresAt time.Time
	if iat, ok := claims["iat"].(float64); ok {
		issuedAt = time.Unix(int64(iat), 0).UTC()
	}
	if exp, ok := claims["exp"].(float64); ok {
		expiresAt = time.Unix(int64(exp), 0).UTC()
	}

	return AuthResult{
		Valid: true,
		Claims: TokenClaims{
			TokenID:   tokenID,
			TenantID:  tenantID,
			Role:      Role(roleStr),
			TokenType: TokenType(tokenTypeStr),
			IssuedAt:  issuedAt,
			ExpiresAt: expiresAt,
		},
	}
}

// IssueJWT creates a signed JWT for the given claims. Returns the signed token.
// secret must be a high-entropy value — never log or store it in plaintext.
func IssueJWT(tokenID, tenantID string, role Role, ttl time.Duration, secret string) (string, error) {
	if tokenID == "" || tenantID == "" || secret == "" {
		return "", fmt.Errorf("tokenID, tenantID, and secret are required")
	}
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"token_id":   tokenID,
		"tenant_id":  tenantID,
		"role":       string(role),
		"token_type": string(TokenTypeJWT),
		"iat":        now.Unix(),
		"exp":        now.Add(ttl).Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	return signed, nil
}

// GenerateAPIToken creates a cryptographically random, tenant-bound API token.
// Format: "autostack_<tenantPrefix>_<random32hex>"
// The full token is returned exactly once — store its hash, not the token.
func GenerateAPIToken(tenantID string) (token, tokenHash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate token entropy: %w", err)
	}
	prefix := strings.ReplaceAll(tenantID, "-", "")
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	token = fmt.Sprintf("autostack_%s_%s", prefix, hex.EncodeToString(b))
	tokenHash = hashAPIToken(token)
	return token, tokenHash, nil
}

// VerifyAPIToken verifies a raw API token against its stored hash.
// Constant-time comparison — safe against timing attacks.
func VerifyAPIToken(rawToken, storedHash string) bool {
	if rawToken == "" || storedHash == "" {
		return false
	}
	computed := hashAPIToken(rawToken)
	return hmac.Equal([]byte(computed), []byte(storedHash))
}

func hashAPIToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
