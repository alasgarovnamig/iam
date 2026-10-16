// Package httpapi is the inbound HTTP adapter: routing, middleware, DTO
// mapping and response rendering. No business logic lives here.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/sr-alasgarov/wallet-gateway/internal/apperr"
)

type responseMeta struct {
	RequestID string    `json:"request_id"`
	Timestamp time.Time `json:"timestamp"`
}

type successEnvelope struct {
	Data any          `json:"data"`
	Meta responseMeta `json:"meta"`
}

type paginationInfo struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type listEnvelope struct {
	Data       any            `json:"data"`
	Pagination paginationInfo `json:"pagination"`
	Meta       responseMeta   `json:"meta"`
}

// problem is the RFC 7807 error body.
type problem struct {
	Type      string              `json:"type"`
	Title     string              `json:"title"`
	Status    int                 `json:"status"`
	Detail    string              `json:"detail"`
	Code      string              `json:"code"`
	RequestID string              `json:"request_id"`
	Errors    []apperr.FieldError `json:"errors,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondData(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeJSON(w, status, successEnvelope{
		Data: data,
		Meta: responseMeta{RequestID: requestIDFrom(r.Context()), Timestamp: time.Now().UTC()},
	})
}

func respondList(w http.ResponseWriter, r *http.Request, data any, page, pageSize, total int) {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	writeJSON(w, http.StatusOK, listEnvelope{
		Data:       data,
		Pagination: paginationInfo{Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages},
		Meta:       responseMeta{RequestID: requestIDFrom(r.Context()), Timestamp: time.Now().UTC()},
	})
}

func respondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func respondError(w http.ResponseWriter, r *http.Request, err error, logger *slog.Logger) {
	app := apperr.From(err)
	if app.Status >= 500 {
		logger.Error("request failed", "error", err, "request_id", requestIDFrom(r.Context()),
			"path", r.URL.Path, "method", r.Method)
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(app.Status)
		return
	}
	body := problem{
		Type:      "https://wallet-gateway/errors/" + kebab(string(app.Code)),
		Title:     app.Title,
		Status:    app.Status,
		Detail:    app.Detail,
		Code:      string(app.Code),
		RequestID: requestIDFrom(r.Context()),
		Errors:    app.Fields,
	}
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(app.Status)
	_ = json.NewEncoder(w).Encode(body)
}

func kebab(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '_' {
			out = append(out, '-')
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}
