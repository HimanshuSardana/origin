package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/HimanshuSardana/origin/api"
	"github.com/HimanshuSardana/origin/whatsapp"
)

func main() {
	client, err := whatsapp.NewClient("origin.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	server := api.NewServer(client)
	server.RegisterRoutes(mux)

	addr := ":" + port
	fmt.Printf("Origin API server starting on http://localhost%s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
