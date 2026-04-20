package server

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOpenModeAllowsAccessWithoutLogin(t *testing.T) {
	t.Parallel()
	s := newE2ETestServer(t, t.TempDir(), "http://127.0.0.1:0")
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected open mode to allow request, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProtectedModeRedirectsHTMLRequestsToLogin(t *testing.T) {
	t.Parallel()
	s := newProtectedAuthTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect to login, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/login?next=%2Fsettings" {
		t.Fatalf("expected login redirect, got %q", got)
	}
}

func TestProtectedModeRejectsAPIWithoutSession(t *testing.T) {
	t.Parallel()
	s := newProtectedAuthTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "需要登入") {
		t.Fatalf("expected auth error body, got %q", rec.Body.String())
	}
}

func TestProtectedModeRejectsWrongPassword(t *testing.T) {
	t.Parallel()
	s := newProtectedAuthTestServer(t)
	form := url.Values{
		"password": {"wrong-pass"},
		"next":     {"/settings"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "密碼錯誤") {
		t.Fatalf("expected error message in body, got %q", rec.Body.String())
	}
}

func TestProtectedModeLoginAndLogoutFlow(t *testing.T) {
	t.Parallel()
	s := newProtectedAuthTestServer(t)
	form := url.Values{
		"password": {"secret-pass"},
		"next":     {"/settings"},
	}
	loginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("Accept", "text/html")
	loginRec := httptest.NewRecorder()
	s.router.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusSeeOther {
		t.Fatalf("expected login redirect, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	if got := loginRec.Header().Get("Location"); got != "/settings" {
		t.Fatalf("expected redirect to requested page, got %q", got)
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie to be set")
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	apiReq.Header.Set("Accept", "application/json")
	apiReq.AddCookie(cookies[0])
	apiRec := httptest.NewRecorder()
	s.router.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("expected authenticated API access, got %d: %s", apiRec.Code, apiRec.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/logout", nil)
	logoutReq.Header.Set("Accept", "application/json")
	logoutReq.AddCookie(cookies[0])
	logoutRec := httptest.NewRecorder()
	s.router.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("expected logout success, got %d", logoutRec.Code)
	}

	afterLogoutReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	afterLogoutReq.Header.Set("Accept", "application/json")
	afterLogoutReq.AddCookie(cookies[0])
	afterLogoutRec := httptest.NewRecorder()
	s.router.ServeHTTP(afterLogoutRec, afterLogoutReq)
	if afterLogoutRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected session to be invalid after logout, got %d", afterLogoutRec.Code)
	}
}

func newProtectedAuthTestServer(t *testing.T) *Server {
	t.Helper()
	s := newE2ETestServer(t, t.TempDir(), "http://127.0.0.1:0")
	s.cfg.AuthMode = "password"
	s.cfg.AuthPassword = "secret-pass"
	s.auth = newAuthManager(s.cfg)
	s.router = setupAuthTestRouter(s)
	return s
}

func setupAuthTestRouter(s *Server) *gin.Engine {
	s.router = gin.New()
	s.router.SetHTMLTemplate(template.Must(template.New("login.html").Parse(`{{define "login.html"}}{{.Error}}{{end}}`)))
	s.setupRoutes()
	return s.router
}
