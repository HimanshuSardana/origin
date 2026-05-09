package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/HimanshuSardana/origin/whatsapp"
)

type Server struct {
	client *whatsapp.Client
}

func NewServer(client *whatsapp.Client) *Server {
	return &Server{client: client}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /contacts", s.handleContacts)
	mux.HandleFunc("GET /messages", s.handleMessages)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	contacts, err := s.client.GetContacts()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to fetch contacts: %v", err))
		return
	}
	respondJSON(w, http.StatusOK, contacts)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	jid := r.URL.Query().Get("jid")
	if jid == "" {
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
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to fetch messages: %v", err))
		return
	}

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
