package auth

import (
	"testing"
	"time"
)

func TestIssueAndParseExternalToken(t *testing.T) {
	secret := "shared-secret-app1"
	userID := "user-123"
	email := "u@example.com"
	name := "Alice"
	audience := "app-client-1"

	token, err := IssueExternalToken(secret, ExternalTokenIssuer, userID, email, name, audience, "read", "proj-1", "task-1", 5*time.Minute)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}

	claims, err := ParseExternalToken(secret, token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.Subject != userID {
		t.Errorf("subject = %q, want %q", claims.Subject, userID)
	}
	if claims.UserID != userID {
		t.Errorf("user_id = %q, want %q", claims.UserID, userID)
	}
	if claims.Email != email || claims.Name != name {
		t.Errorf("email/name not carried: %q / %q", claims.Email, claims.Name)
	}
	if claims.Scope != "read" || claims.ProjectID != "proj-1" || claims.TaskID != "task-1" {
		t.Errorf("scope/context not carried: %+v", claims)
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != audience {
		t.Errorf("audience = %v, want [%q]", claims.Audience, audience)
	}
	if claims.Issuer != ExternalTokenIssuer {
		t.Errorf("issuer = %q, want %q", claims.Issuer, ExternalTokenIssuer)
	}
	if claims.ID == "" {
		t.Error("ID (nonce) is empty")
	}
}

func TestParseExternalTokenWrongSecret(t *testing.T) {
	token, err := IssueExternalToken("secret-A", ExternalTokenIssuer, "u", "", "", "app1", "", "", "", time.Minute)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := ParseExternalToken("secret-B", token); err == nil {
		t.Fatal("expected parse error with wrong secret")
	}
}

func TestParseExternalTokenExpired(t *testing.T) {
	token, err := IssueExternalToken("secret-A", ExternalTokenIssuer, "u", "", "", "app1", "", "", "", -time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := ParseExternalToken("secret-A", token); err == nil {
		t.Fatal("expected parse error for expired token")
	}
}

func TestIssueExternalTokenEmptySecret(t *testing.T) {
	if _, err := IssueExternalToken("", ExternalTokenIssuer, "u", "", "", "app1", "", "", "", time.Minute); err == nil {
		t.Fatal("expected error for empty secret")
	}
}
