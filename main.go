package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/HimanshuSardana/origin/api"
	"github.com/HimanshuSardana/origin/whatsapp"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	client, err := whatsapp.NewClient("origin.db")
	if err != nil {
		logger.Error("failed to create client", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer client.Close()
	logger.Info("whatsapp client initialized", slog.String("db", "origin.db"))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	server := api.NewServer(client, logger)
	server.RegisterRoutes(mux)

	addr := ":" + port
	logger.Info("starting server", slog.String("addr", addr))
	if err := http.ListenAndServe(addr, server.LoggingMiddleware(mux)); err != nil {
		logger.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
