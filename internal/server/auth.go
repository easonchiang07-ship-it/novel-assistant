package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionCookieName = "novel_assistant_session"

type authManager struct {
	enabled       bool
	password      string
	cookieName    string
	secureCookies bool
	sessionTTL    time.Duration

	mu       sync.Mutex
	sessions map[string]time.Time
}

func newAuthManager(cfg authConfig) *authManager {
	manager := &authManager{
		cookieName: sessionCookieName,
		sessionTTL: 7 * 24 * time.Hour,
		sessions:   map[string]time.Time{},
	}
	if cfg == nil || !cfg.AuthEnabled() {
		return manager
	}
	manager.enabled = true
	manager.password = cfg.GetAuthPassword()
	manager.secureCookies = cfg.GetAuthCookieSecure()
	return manager
}

type authConfig interface {
	AuthEnabled() bool
	GetAuthPassword() string
	GetAuthCookieSecure() bool
}

func (a *authManager) Enabled() bool {
	return a != nil && a.enabled
}

func (a *authManager) createSession() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	a.mu.Lock()
	a.sessions[token] = time.Now().Add(a.sessionTTL)
	a.mu.Unlock()
	return token, nil
}

func (a *authManager) verifySession(token string) bool {
	if !a.Enabled() {
		return true
	}
	if token == "" {
		return false
	}
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	expiry, ok := a.sessions[token]
	if !ok {
		return false
	}
	if now.After(expiry) {
		delete(a.sessions, token)
		return false
	}
	return true
}

func (a *authManager) destroySession(token string) {
	if a == nil || token == "" {
		return
	}
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

func (a *authManager) checkPassword(password string) bool {
	if !a.Enabled() {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) == 1
}

func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.auth == nil || !s.auth.Enabled() {
			c.Next()
			return
		}
		token, err := c.Cookie(s.auth.cookieName)
		if err == nil && s.auth.verifySession(token) {
			c.Next()
			return
		}
		if wantsHTML(c) {
			target := "/login"
			if path := currentRequestPath(c); path != "" && path != "/login" {
				target += "?next=" + url.QueryEscape(path)
			}
			c.Redirect(http.StatusSeeOther, target)
			c.Abort()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "需要登入"})
	}
}

func (s *Server) handleLoginPage(c *gin.Context) {
	if s.auth == nil || !s.auth.Enabled() {
		c.Redirect(http.StatusSeeOther, "/")
		return
	}
	if token, err := c.Cookie(s.auth.cookieName); err == nil && s.auth.verifySession(token) {
		c.Redirect(http.StatusSeeOther, sanitizeNextPath(c.Query("next")))
		return
	}
	c.HTML(http.StatusOK, "login.html", gin.H{
		"Title": "登入",
		"Next":  sanitizeNextPath(c.Query("next")),
	})
}

func (s *Server) handleLogin(c *gin.Context) {
	if s.auth == nil || !s.auth.Enabled() {
		c.Redirect(http.StatusSeeOther, "/")
		return
	}
	password := c.PostForm("password")
	next := sanitizeNextPath(c.PostForm("next"))
	if !s.auth.checkPassword(password) {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"Title": "登入",
			"Next":  next,
			"Error": "密碼錯誤，請再試一次。",
		})
		return
	}
	token, err := s.auth.createSession()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{
			"Title": "登入",
			"Next":  next,
			"Error": "建立登入 session 失敗，請稍後再試。",
		})
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(s.auth.cookieName, token, int(s.auth.sessionTTL.Seconds()), "/", "", s.auth.secureCookies, true)
	c.Redirect(http.StatusSeeOther, next)
}

func (s *Server) handleLogout(c *gin.Context) {
	if s.auth != nil {
		if token, err := c.Cookie(s.auth.cookieName); err == nil {
			s.auth.destroySession(token)
		}
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(s.auth.cookieName, "", -1, "/", "", s.auth.secureCookies, true)
	}
	if wantsHTML(c) {
		target := "/"
		if s.auth != nil && s.auth.Enabled() {
			target = "/login"
		}
		c.Redirect(http.StatusSeeOther, target)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func currentRequestPath(c *gin.Context) string {
	path := c.Request.URL.Path
	if raw := c.Request.URL.RawQuery; raw != "" {
		path += "?" + raw
	}
	return path
}

func sanitizeNextPath(next string) string {
	next = strings.TrimSpace(next)
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	return next
}

func wantsHTML(c *gin.Context) bool {
	accept := c.GetHeader("Accept")
	return strings.Contains(accept, "text/html")
}
