package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
)

const tokenPrefix = "Bearer "

func main() {
	address := flag.String("address", ":8080", "HTTP address")
	service := flag.String("service", "user", "service identity")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !hasToken(r.Header.Get("Authorization")) {
			http.Error(w, "missing client token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case *service == "user" && r.URL.Path == "/users/42":
			writeJSON(w, map[string]any{"id": 42, "user_status": "ACTIVE", "date_create": "2024-03-15T10:22:00Z"})
		case *service == "ledger" && r.URL.Path == "/ledger/accounts/42":
			writeJSON(w, map[string]any{"account_number": "A-100", "ledger_balance": 1580.42})
		case *service == "account" && r.URL.Path == "/accounts/42/flags":
			writeJSON(w, map[string]any{"has_pnd": true, "has_lien": true})
		default:
			http.NotFound(w, r)
		}
	})

	server := &http.Server{Addr: *address, Handler: mux}
	log.Printf("%s listening on %s", *service, *address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func hasToken(value string) bool {
	return strings.HasPrefix(value, tokenPrefix) && len(strings.TrimSpace(strings.TrimPrefix(value, tokenPrefix))) > 0
}

func writeJSON(w http.ResponseWriter, value any) {
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}
