package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"reports-api/internal/auth"
	"reports-api/internal/config"
	"reports-api/internal/report"
	"reports-api/internal/storage"
)

type Server struct {
	cfg     config.Config
	auth    *auth.Client
	reports *report.Service
	store   *storage.S3Client
}

type reportResponse struct {
	URL       string `json:"url"`
	Cached    bool   `json:"cached"`
	ObjectKey string `json:"object_key"`
}

func NewServer(cfg config.Config, authClient *auth.Client, reports *report.Service, store *storage.S3Client) *Server {
	return &Server{
		cfg:     cfg,
		auth:    authClient,
		reports: reports,
		store:   store,
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

	query := report.Query{
		Username: identity.Username,
		DateFrom: dateFrom,
		DateTo:   dateTo,
	}

	objectKey := reportObjectKey(query)

	exists, err := s.store.HeadObject(r.Context(), objectKey)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !exists {
		csvData, err := s.reports.CSV(r.Context(), query)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		filename := fmt.Sprintf("report_%s_%s_%s.csv", identity.Username, dateFrom.Format("20060102"), dateTo.Format("20060102"))
		err = s.store.PutObject(r.Context(), objectKey, csvData, "text/csv; charset=utf-8", map[string]string{
			"Cache-Control":       "public, max-age=300, s-maxage=86400, immutable",
			"Content-Disposition": fmt.Sprintf("attachment; filename=%q", filename),
		})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, reportResponse{
		URL:       s.cdnURL(objectKey),
		Cached:    exists,
		ObjectKey: objectKey,
	})
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
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	if len(methods) > 0 {
		w.Header().Set("Allow", strings.Join(methods, ", "))
	}
	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func reportObjectKey(query report.Query) string {
	return path.Join(
		query.Username,
		query.DateFrom.UTC().Format("2006-01-02")+"_"+query.DateTo.UTC().Format("2006-01-02"),
		"report.csv",
	)
}

func (s *Server) cdnURL(objectKey string) string {
	base := strings.TrimRight(s.cfg.CDNBaseURL, "/")
	return base + "/" + objectKey
}
