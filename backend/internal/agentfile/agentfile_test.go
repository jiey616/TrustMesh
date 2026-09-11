package agentfile

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"trustmesh/backend/internal/auth"
)

const testSecret = "test-secret-for-download-tokens"

func newTestHandler() *Handler {
	return NewHandler(nil, nil, auth.NewJWTManager(testSecret, time.Hour, time.Hour), zap.NewNop())
}

func newTestRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/files/agent/:fileId/token/:token", h.Download)
	r.GET("/files/agent/:fileId", h.Download)
	return r
}

// signToken mints a download token with an arbitrary expiry. GenerateDownloadToken
// clamps non-positive TTLs to the default, so an already-expired token has to be
// signed directly.
func signToken(t *testing.T, fileID string, issuedAt, expiresAt time.Time) string {
	t.Helper()
	claims := auth.Claims{
		UserID:    fileID,
		TokenType: tokenTypeAgentDownload,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// TestDownloadTokenExpiredIsDetectable pins the assumption the whole P-04 fix
// rests on: the JWT library must return an error that errors.Is can match
// against ErrTokenExpired. If a library upgrade ever breaks this, the
// DOWNLOAD_TOKEN_EXPIRED branch silently stops firing and agents go back to
// treating a recoverable error as fatal.
func TestDownloadTokenExpiredIsDetectable(t *testing.T) {
	mgr := auth.NewJWTManager(testSecret, time.Hour, time.Hour)
	expired := signToken(t, "file-1", time.Now().UTC().Add(-2*time.Hour), time.Now().UTC().Add(-time.Hour))

	_, err := mgr.ParseToken(expired)
	if err == nil {
		t.Fatalf("expired token must fail to parse")
	}
	if !errors.Is(err, jwt.ErrTokenExpired) {
		t.Fatalf("errors.Is(err, jwt.ErrTokenExpired) = false, err = %v", err)
	}
}

// TestDownloadExpiredTokenReturnsGuidance is acceptance criterion D2: an
// expired token must hand the agent a machine-readable code plus the recovery
// instruction, instead of the old "invalid or expired" dead end.
func TestDownloadExpiredTokenReturnsGuidance(t *testing.T) {
	h := newTestHandler()
	r := newTestRouter(h)

	tok := signToken(t, "file-1", time.Now().UTC().Add(-2*time.Hour), time.Now().UTC().Add(-time.Hour))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/files/agent/file-1/token/"+tok, nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expired token status = %d, want 401", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "DOWNLOAD_TOKEN_EXPIRED") {
		t.Fatalf("body must expose code DOWNLOAD_TOKEN_EXPIRED, got %s", body)
	}
	if !strings.Contains(body, "task.context.query") {
		t.Fatalf("body must tell the agent how to recover, got %s", body)
	}
}

// TestDownloadInvalidTokenStaysDistinct guards the other half: a genuinely bad
// token must NOT claim to be expired, otherwise agents would retry a link that
// can never work.
func TestDownloadInvalidTokenStaysDistinct(t *testing.T) {
	h := newTestHandler()
	r := newTestRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/files/agent/file-1/token/not-a-jwt", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status = %d, want 401", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "DOWNLOAD_TOKEN_EXPIRED") {
		t.Fatalf("invalid token must not be reported as expired, got %s", body)
	}
	if !strings.Contains(body, "invalid download token") {
		t.Fatalf("body must report an invalid token, got %s", body)
	}
}

// TestDownloadMissingTokenRejected keeps the existing contract intact.
func TestDownloadMissingTokenRejected(t *testing.T) {
	h := newTestHandler()
	r := newTestRouter(h)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/files/agent/file-1", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want 401", w.Code)
	}
}

// TestDefaultDownloadTTLOutlastsSlowSteps locks in the P-04 TTL floor: agent
// download links must survive the slowest real step (video generation was
// measured well over an hour in production).
func TestDefaultDownloadTTLOutlastsSlowSteps(t *testing.T) {
	if defaultDownloadTTL < time.Hour {
		t.Fatalf("defaultDownloadTTL = %v, must be at least 1h", defaultDownloadTTL)
	}
	tok, err := GenerateDownloadToken([]byte(testSecret), "file-1", 0)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	mgr := auth.NewJWTManager(testSecret, time.Hour, time.Hour)
	if _, err := mgr.ParseToken(tok); err != nil {
		t.Fatalf("freshly minted token must parse: %v", err)
	}
}
