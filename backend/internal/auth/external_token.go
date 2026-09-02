package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ExternalTokenIssuer is the `iss` claim embedded in externally issued SSO
// tokens. External platforms must configure this as the expected issuer.
const ExternalTokenIssuer = "trustmesh"

// ExternalClaims is the payload of an SSO token handed to an external platform
// when launching it from TrustMesh.
type ExternalClaims struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
	Scope     string `json:"scope,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	jwt.RegisteredClaims
}

// IssueExternalToken signs an HS256 JWT scoped to a single external app.
//
//   - secret   : the app's client_secret (shared with the external platform)
//   - issuer   : expected `iss` (use ExternalTokenIssuer)
//   - audience : the app's client_id (the external platform must reject tokens
//     whose `aud` does not match its own client_id)
func IssueExternalToken(secret, issuer, userID, email, name, audience, scope, projectID, taskID string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("external app secret is empty")
	}
	if audience == "" {
		return "", errors.New("external app audience (client_id) is empty")
	}
	now := time.Now().UTC()
	claims := ExternalClaims{
		UserID:    userID,
		Email:     email,
		Name:      name,
		Scope:     scope,
		ProjectID: projectID,
		TaskID:    taskID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        genNonce(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseExternalToken verifies and decodes an external SSO token using the
// app's shared client_secret.
func ParseExternalToken(secret, tokenStr string) (*ExternalClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &ExternalClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*ExternalClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func genNonce() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(buf)
}
