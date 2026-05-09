package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/HimanshuSardana/origin/whatsapp"
)

type Server struct {
	client *whatsapp.Client
	logger *slog.Logger
}

func NewServer(client *whatsapp.Client, logger *slog.Logger) *Server {
	return &Server{client: client, logger: logger}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /contacts", s.handleContacts)
	mux.HandleFunc("GET /messages", s.handleMessages)
}

func (s *Server) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		s.logger.Info("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("query", r.URL.RawQuery),
			slog.String("remote_addr", r.RemoteAddr),
			slog.Int("status", wrapped.statusCode),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	contacts, err := s.client.GetContacts()
	if err != nil {
		s.logger.Error("failed to fetch contacts", slog.String("error", err.Error()))
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to fetch contacts: %v", err))
		return
	}
	s.logger.Debug("contacts fetched", slog.Int("count", len(contacts)))
	respondJSON(w, http.StatusOK, contacts)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	jid := r.URL.Query().Get("jid")
	if jid == "" {
		s.logger.Warn("missing jid parameter")
		respondError(w, http.StatusBadRequest, "missing 'jid' query parameter")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err == nil && parsed > 0 {
			limit = parsed
		}
	}

	messages, err := s.client.GetMessagesFromDB(jid, limit)
	if err != nil {
		s.logger.Error("failed to fetch messages",
			slog.String("jid", jid),
			slog.String("error", err.Error()),
		)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to fetch messages: %v", err))
		return
	}

	if messages == nil {
		messages = []whatsapp.Message{}
	}

	s.logger.Debug("messages fetched",
		slog.String("jid", jid),
		slog.Int("count", len(messages)),
		slog.Int("limit", limit),
	)
	respondJSON(w, http.StatusOK, messages)
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

type errorResponse struct {
	Error string `json:"error"`
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, errorResponse{Error: message})
}
