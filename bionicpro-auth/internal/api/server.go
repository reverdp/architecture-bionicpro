package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"bionicpro-auth/internal/config"
	"bionicpro-auth/internal/keycloak"
	"bionicpro-auth/internal/profile"
	"bionicpro-auth/internal/session"
)

type Server struct {
	cfg      config.Config
	store    *session.Store
	keycloak *keycloak.Client
	profiles *profile.Store
}

const sessionIDHeader = "X-Session-Id"

func NewServer(cfg config.Config, store *session.Store, keycloakClient *keycloak.Client, profileStore *profile.Store) *Server {
	server := &Server{
		cfg:      cfg,
		store:    store,
		keycloak: keycloakClient,
		profiles: profileStore,
	}

	go server.cleanupLoop(context.Background())
	return server
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/login", s.handleLogin)
	mux.HandleFunc("/auth/callback", s.handleCallback)
	mux.HandleFunc("/auth/session", s.handleSession)
	mux.HandleFunc("/auth/profile", s.handleProfile)
	mux.HandleFunc("/auth/refresh", s.handleRefresh)
	mux.HandleFunc("/auth/logout", s.handleLogout)

	return s.withJSON(mux)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	authURL, err := s.prepareLogin(w, r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) prepareLogin(w http.ResponseWriter, r *http.Request) (string, error) {
	current, ok := s.currentSession(r)
	if !ok {
		var err error
		current, err = s.store.Create()
		if err != nil {
			return "", err
		}
	}

	state, err := sessionToken()
	if err != nil {
		return "", err
	}
	verifier, err := sessionToken()
	if err != nil {
		return "", err
	}

	if err := s.store.SetPendingAuth(current.ID, state, verifier); err != nil {
		return "", err
	}

	s.setSessionCookie(w, current.ID)

	return s.keycloak.BuildAuthorizationURL(state, keycloak.PKCEChallenge(verifier), s.identityProviderHint(r)), nil
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	current, ok := s.currentSession(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "session not found")
		return
	}

	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		writeError(w, http.StatusBadRequest, "state and code are required")
		return
	}
	if !s.store.StateMatches(current.ID, state) {
		writeError(w, http.StatusUnauthorized, "invalid session state")
		return
	}

	verifier, ok := s.store.CodeVerifier(current.ID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing code verifier")
		return
	}

	tokenResp, err := s.keycloak.ExchangeCode(r.Context(), code, verifier)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	saved, err := s.store.SaveTokens(current.ID, session.TokenSet{
		AccessToken:      tokenResp.AccessToken,
		RefreshToken:     tokenResp.RefreshToken,
		TokenType:        tokenResp.TokenType,
		ExpiresIn:        tokenResp.ExpiresIn,
		RefreshExpiresIn: tokenResp.RefreshExpiresIn,
		Subject:          keycloak.ParseSubject(tokenResp.AccessToken),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.syncYandexProfile(r.Context(), saved.Subject, tokenResp.AccessToken); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	rotated, err := s.rotateSession(w, saved, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.cfg.FrontendURL != "" {
		http.Redirect(w, r, callbackRedirectURL(s.cfg.FrontendURL, rotated.ID), http.StatusFound)
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse(rotated))
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	current, ok := s.currentSession(r)
	if !ok || !current.Authenticated || current.Subject == "" {
		writeError(w, http.StatusUnauthorized, "session not found")
		return
	}

	rec, err := s.profiles.GetBySubject(r.Context(), current.Subject)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rec == nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	current, ok := s.currentSession(r)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	rotated, err := s.rotateSession(w, current, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sessionResponse(rotated))
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	current, ok := s.currentSession(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "session not found")
		return
	}

	refreshToken, err := s.store.RefreshToken(current.ID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	tokenResp, err := s.keycloak.Refresh(r.Context(), refreshToken)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	saved, err := s.store.SaveTokens(current.ID, session.TokenSet{
		AccessToken:      tokenResp.AccessToken,
		RefreshToken:     tokenResp.RefreshToken,
		TokenType:        tokenResp.TokenType,
		ExpiresIn:        tokenResp.ExpiresIn,
		RefreshExpiresIn: tokenResp.RefreshExpiresIn,
		Subject:          keycloak.ParseSubject(tokenResp.AccessToken),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rotated, err := s.rotateSession(w, saved, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sessionResponse(rotated))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	current, ok := s.currentSession(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "session not found")
		return
	}

	refreshToken, err := s.store.RefreshToken(current.ID)
	if err == nil {
		_ = s.keycloak.Logout(r.Context(), refreshToken)
	}

	s.store.Delete(current.ID)
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *Server) withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.applyCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) applyCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" || s.cfg.FrontendURL == "" || origin != s.cfg.FrontendURL {
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Expose-Headers", sessionIDHeader)
	w.Header().Set("Vary", "Origin")
}

func (s *Server) currentSession(r *http.Request) (*session.Session, bool) {
	cookie, err := r.Cookie(s.cfg.SessionCookie)
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	return s.store.Get(cookie.Value)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookie,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
}

func (s *Server) rotateSession(w http.ResponseWriter, current *session.Session, force bool) (*session.Session, error) {
	if current == nil {
		return nil, fmt.Errorf("session not found")
	}

	if !current.Authenticated {
		s.setSessionCookie(w, current.ID)
		w.Header().Set(sessionIDHeader, current.ID)
		return current, nil
	}

	if !force && !s.shouldRotate(current) {
		s.setSessionCookie(w, current.ID)
		w.Header().Set(sessionIDHeader, current.ID)
		return current, nil
	}

	rotated, err := s.store.Rotate(current.ID)
	if err != nil {
		return nil, err
	}

	s.setSessionCookie(w, rotated.ID)
	w.Header().Set(sessionIDHeader, rotated.ID)
	return rotated, nil
}

func (s *Server) shouldRotate(current *session.Session) bool {
	if current.AccessExpiresAt.IsZero() {
		return false
	}

	return time.Now().UTC().Before(current.AccessExpiresAt)
}

func (s *Server) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.store.CleanupExpired()
		}
	}
}

func sessionResponse(current *session.Session) map[string]any {
	return map[string]any{
		"session_id":         current.ID,
		"authenticated":      current.Authenticated,
		"subject":            current.Subject,
		"token_type":         current.TokenType,
		"has_access_token":   current.AccessToken != "",
		"has_refresh_token":  len(current.RefreshTokenCiphertext) > 0,
		"access_expires_at":  current.AccessExpiresAt,
		"refresh_expires_at": current.RefreshExpiresAt,
		"created_at":         current.CreatedAt,
		"updated_at":         current.UpdatedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", headerMethods(methods))
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func headerMethods(methods []string) string {
	if len(methods) == 0 {
		return ""
	}
	result := methods[0]
	for i := 1; i < len(methods); i++ {
		result += ", " + methods[i]
	}
	return result
}

func sessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func callbackRedirectURL(frontendURL, sessionID string) string {
	parsed, err := url.Parse(frontendURL)
	if err != nil {
		return frontendURL
	}

	query := parsed.Query()
	query.Set("auth", "success")
	if sessionID != "" {
		query.Set("session_id", sessionID)
	}
	parsed.RawQuery = query.Encode()

	return fmt.Sprintf("%s", parsed)
}

func (s *Server) identityProviderHint(r *http.Request) string {
	idp := r.URL.Query().Get("idp")
	if idp != "" {
		return idp
	}
	return s.cfg.DefaultIDPAlias
}

func (s *Server) syncYandexProfile(ctx context.Context, subject, keycloakAccessToken string) error {
	if s.profiles == nil || subject == "" || keycloakAccessToken == "" {
		return nil
	}

	claims, err := keycloak.ParseClaims(keycloakAccessToken)
	if err != nil {
		return fmt.Errorf("parse keycloak access token claims: %w", err)
	}

	raw, err := json.Marshal(claims)
	if err != nil {
		return fmt.Errorf("marshal keycloak claims: %w", err)
	}

	err = s.profiles.SaveYandexProfile(ctx, subject, s.identityProviderHintFromClaims(), profile.YandexProfile{
		ID:             claims.Sub,
		Username:       claims.PreferredUsername,
		DisplayName:    claims.Name,
		FirstName:      claims.GivenName,
		LastName:       claims.FamilyName,
		Email:          claims.Email,
		EmailVerified:  claims.EmailVerified,
		Scope:          claims.Scope,
		Audience:       append(json.RawMessage(nil), encodeAudience(claims.Audience)...),
		AllowedOrigins: append([]string(nil), claims.AllowedOrigins...),
		RawJSON:        append(json.RawMessage(nil), raw...),
	})
	if err != nil {
		return fmt.Errorf("persist profile: %w", err)
	}

	log.Printf("synced broker profile for subject=%s username=%s", subject, claims.PreferredUsername)
	return nil
}

func (s *Server) identityProviderHintFromClaims() string {
	if s.cfg.DefaultIDPAlias != "" {
		return s.cfg.DefaultIDPAlias
	}
	return "broker"
}

func encodeAudience(aud any) []byte {
	raw, err := json.Marshal(aud)
	if err != nil || len(raw) == 0 {
		return []byte("null")
	}
	return raw
}
