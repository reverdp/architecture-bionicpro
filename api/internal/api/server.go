package api

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"reports-api/internal/auth"
	"reports-api/internal/config"
	"reports-api/internal/report"
)

type Server struct {
	cfg     config.Config
	auth    *auth.Client
	reports *report.Service
}

func NewServer(cfg config.Config, authClient *auth.Client, reports *report.Service) *Server {
	return &Server{
		cfg:     cfg,
		auth:    authClient,
		reports: reports,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/reports", s.handleReports)
	return s.withCORS(mux)
}

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	sessionID, err := sessionCookieValue(r, s.cfg.AuthSessionCookie)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, err.Error())
		return
	}

	identity, err := s.auth.ResolveSession(r.Context(), sessionID)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, err.Error())
		return
	}

	dateFrom, dateTo, err := parsePeriod(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	filename := fmt.Sprintf("report_%s_%s_%s.csv", identity.Username, dateFrom.Format("20060102"), dateTo.Format("20060102"))
	var buf bytes.Buffer
	err = s.reports.WriteCSV(r.Context(), &buf, report.Query{
		Username: identity.Username,
		DateFrom: dateFrom,
		DateTo:   dateTo,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin == s.cfg.FrontendURL {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func sessionCookieValue(r *http.Request, cookieName string) (string, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", fmt.Errorf("session cookie is required")
	}
	return strings.TrimSpace(cookie.Value), nil
}

func parsePeriod(r *http.Request) (time.Time, time.Time, error) {
	dateFromRaw := r.URL.Query().Get("date_from")
	dateToRaw := r.URL.Query().Get("date_to")
	if dateFromRaw == "" || dateToRaw == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("date_from and date_to are required in YYYY-MM-DD format")
	}

	dateFrom, err := time.Parse("2006-01-02", dateFromRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid date_from, expected YYYY-MM-DD")
	}
	dateTo, err := time.Parse("2006-01-02", dateToRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid date_to, expected YYYY-MM-DD")
	}
	if dateTo.Before(dateFrom) {
		return time.Time{}, time.Time{}, fmt.Errorf("date_to must be greater than or equal to date_from")
	}

	from := dateFrom.UTC()
	to := dateTo.Add(24*time.Hour - time.Nanosecond).UTC()
	return from, to, nil
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"error":%q}`, message)))
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	if len(methods) > 0 {
		w.Header().Set("Allow", strings.Join(methods, ", "))
	}
	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
}
