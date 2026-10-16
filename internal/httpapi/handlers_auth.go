package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/sr-alasgarov/wallet-gateway/internal/app"
	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

// decodeBody decodes JSON strictly and returns the set of top-level keys the
// client actually sent (mass-assignment guard input).
func decodeBody(r *http.Request, dst any) ([]string, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, apperr.BadRequest("Cannot read request body.")
	}
	if len(raw) == 0 {
		return nil, apperr.BadRequest("Request body is required.")
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, apperr.BadRequest("Request body must be a JSON object.")
	}
	provided := make([]string, 0, len(keys))
	for k := range keys {
		provided = append(provided, k)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if err := dec.Decode(dst); err != nil {
		return nil, apperr.BadRequest("Invalid request body: " + err.Error())
	}
	return provided, nil
}

// parseListPage extracts pagination/sort/filter query parameters. Filters use
// the filter[<field>]=value convention.
func parseListPage(r *http.Request) ports.ListPage {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	// Defaults/clamps mirror the application layer so the pagination block in
	// the response matches what was actually queried.
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	lp := ports.ListPage{
		Page:     page,
		PageSize: pageSize,
		SortBy:   q.Get("sort_by"),
		SortDir:  q.Get("sort_dir"),
		Search:   q.Get("search"),
		Filters:  map[string]string{},
	}
	for key, values := range q {
		if strings.HasPrefix(key, "filter[") && strings.HasSuffix(key, "]") && len(values) > 0 {
			field := key[len("filter[") : len(key)-1]
			lp.Filters[field] = values[0]
		}
	}
	return lp
}

// AuthHandlers serves /api/v1/auth/* and /api/v1/oauth/token.
type AuthHandlers struct {
	Auth        *app.AuthService
	OAuth       *app.OAuthService
	Limiter     ports.RateLimiter
	LoginPerMin int
	// RefreshTTL bounds the refresh cookie lifetime (browser clients).
	RefreshTTL time.Duration
	Logger     *slog.Logger
}

type loginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	TenantSlug string `json:"tenant_slug"`
}

// refreshCookieName holds the refresh token for browser clients.
//
// Browsers opt in with `X-Auth-Mode: cookie`; the token is then returned ONLY
// as a HttpOnly cookie and never in the response body, so script (and any XSS)
// cannot read it. The cookie is scoped to the auth path and SameSite=Strict,
// which keeps cross-site requests from carrying it — the CSRF surface of the
// refresh endpoint. Non-browser clients omit the header and get the token in
// the body as usual.
const refreshCookieName = "wg_refresh_token"

const cookieAuthHeader = "X-Auth-Mode"

func wantsCookieAuth(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get(cookieAuthHeader), "cookie")
}

// setRefreshCookie stores the refresh token in a hardened cookie.
// Secure is enabled whenever the request is not plain-http localhost, so
// local development over http keeps working while deployments get Secure.
func setRefreshCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearRefreshCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// refreshTokenFromRequest prefers the cookie and falls back to the body, so
// both browser and machine clients work against the same endpoint.
func refreshTokenFromRequest(r *http.Request, bodyToken string) string {
	if c, err := r.Cookie(refreshCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	return bodyToken
}

func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	// Brute-force protection: this limiter FAILS CLOSED on Redis errors.
	meta := app.MetaFrom(r.Context())
	ok, err := h.Limiter.Allow(r.Context(), "login:"+meta.IP, h.LoginPerMin, time.Minute)
	if err != nil {
		respondError(w, r, apperr.Unavailable("Login temporarily unavailable."), h.Logger)
		return
	}
	if !ok {
		respondError(w, r, apperr.RateLimited(), h.Logger)
		return
	}
	var req loginRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	pair, err := h.Auth.Login(r.Context(), app.LoginCommand{
		Email: req.Email, Password: req.Password, TenantSlug: req.TenantSlug,
	})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	body := map[string]any{
		"access_token":         pair.AccessToken,
		"token_type":           pair.TokenType,
		"expires_at":           pair.ExpiresAt,
		"must_change_password": pair.MustChangePassword,
		"user":                 userDTO(pair.User, nil, true),
	}
	if wantsCookieAuth(r) {
		setRefreshCookie(w, r, pair.RefreshToken, h.RefreshTTL)
	} else {
		body["refresh_token"] = pair.RefreshToken
	}
	respondData(w, r, http.StatusOK, body)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	// The body is optional in cookie mode.
	_ = json.NewDecoder(r.Body).Decode(&req)

	token := refreshTokenFromRequest(r, req.RefreshToken)
	pair, err := h.Auth.Refresh(r.Context(), token)
	if err != nil {
		// A rejected refresh (expired, reused, revoked) must not leave a stale
		// cookie behind that keeps retrying.
		if wantsCookieAuth(r) || req.RefreshToken == "" {
			clearRefreshCookie(w, r)
		}
		respondError(w, r, err, h.Logger)
		return
	}
	body := map[string]any{
		"access_token":         pair.AccessToken,
		"token_type":           pair.TokenType,
		"expires_at":           pair.ExpiresAt,
		"must_change_password": pair.MustChangePassword,
	}
	// Rotation issues a new refresh token: replace the cookie so the browser
	// never presents the consumed one again (which would look like reuse).
	if wantsCookieAuth(r) || req.RefreshToken == "" {
		setRefreshCookie(w, r, pair.RefreshToken, h.RefreshTTL)
	} else {
		body["refresh_token"] = pair.RefreshToken
	}
	respondData(w, r, http.StatusOK, body)
}

func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req refreshRequest
	// Body optional for logout.
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := h.Auth.Logout(r.Context(), p, refreshTokenFromRequest(r, req.RefreshToken)); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	clearRefreshCookie(w, r)
	respondNoContent(w)
}

func (h *AuthHandlers) LogoutAll(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := h.Auth.LogoutAll(r.Context(), p); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	clearRefreshCookie(w, r)
	respondNoContent(w)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *AuthHandlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	var req changePasswordRequest
	if _, err := decodeBody(r, &req); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	if err := h.Auth.ChangePassword(r.Context(), p, app.ChangePasswordCommand{
		CurrentPassword: req.CurrentPassword, NewPassword: req.NewPassword,
	}); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondNoContent(w)
}

func (h *AuthHandlers) Me(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	user, permissions, err := h.Auth.Me(r.Context(), p)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondData(w, r, http.StatusOK, map[string]any{
		"user":        userDTO(user, nil, true),
		"permissions": permissions,
	})
}

func (h *AuthHandlers) Sessions(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	sessions, err := h.Auth.Sessions(r.Context(), p)
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	out := make([]sessionDTO, len(sessions))
	for i, s := range sessions {
		out[i] = toSessionDTO(s, p.SessionID)
	}
	respondData(w, r, http.StatusOK, out)
}

func (h *AuthHandlers) RevokeSession(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if err := h.Auth.RevokeSession(r.Context(), p, chi.URLParam(r, "sessionId")); err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	respondNoContent(w)
}

type oauthTokenRequest struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// OAuthToken accepts both JSON and form-encoded bodies (RFC 6749 uses forms).
func (h *AuthHandlers) OAuthToken(w http.ResponseWriter, r *http.Request) {
	meta := app.MetaFrom(r.Context())
	ok, err := h.Limiter.Allow(r.Context(), "oauth:"+meta.IP, h.LoginPerMin, time.Minute)
	if err != nil {
		respondError(w, r, apperr.Unavailable("Token endpoint temporarily unavailable."), h.Logger)
		return
	}
	if !ok {
		respondError(w, r, apperr.RateLimited(), h.Logger)
		return
	}
	var req oauthTokenRequest
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		if err := r.ParseForm(); err != nil {
			respondError(w, r, apperr.BadRequest("Invalid form body."), h.Logger)
			return
		}
		req.GrantType = r.PostFormValue("grant_type")
		req.ClientID = r.PostFormValue("client_id")
		req.ClientSecret = r.PostFormValue("client_secret")
	} else {
		if _, err := decodeBody(r, &req); err != nil {
			respondError(w, r, err, h.Logger)
			return
		}
	}
	token, err := h.OAuth.Token(r.Context(), app.ClientCredentialsCommand{
		GrantType: req.GrantType, ClientID: req.ClientID, ClientSecret: req.ClientSecret,
	})
	if err != nil {
		respondError(w, r, err, h.Logger)
		return
	}
	// OAuth responses use the bare RFC 6749 shape, not the platform envelope.
	writeJSON(w, http.StatusOK, token)
}
