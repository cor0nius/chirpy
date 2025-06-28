package main

import (
	"chirpy/internal/database"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Print(err)
	}
	serveMux := http.NewServeMux()
	server := http.Server{Handler: serveMux}
	server.Addr = ":8080"
	apiCfg := &apiConfig{}
	apiCfg.fileserverHits.Store(0)
	apiCfg.dbQueries = database.New(db)
	serveMux.Handle("/app/", http.StripPrefix("/app", apiCfg.middlewareMetricsInc(http.FileServer(http.Dir("app")))))
	serveMux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})
	serveMux.HandleFunc("GET /admin/metrics", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, "<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", apiCfg.fileserverHits.Load())
	})
	serveMux.HandleFunc("POST /admin/reset", func(w http.ResponseWriter, req *http.Request) {
		apiCfg.fileserverHits.Store(0)
	})
	serveMux.HandleFunc("POST /api/validate_chirp", func(w http.ResponseWriter, req *http.Request) {
		type chirp struct {
			Body string `json:"body"`
		}
		decoder := json.NewDecoder(req.Body)
		newChirp := chirp{}
		err := decoder.Decode(&newChirp)
		if err != nil {
			respondWithError(w, 400, "Error decoding chirp")
			return
		}
		if len(newChirp.Body) > 140 {
			respondWithError(w, 400, "Chirp is too long")
		} else {
			resp := struct {
				CleanedBody string `json:"cleaned_body"`
			}{CleanedBody: cleanChirp(newChirp.Body)}
			respondWithJSON(w, 200, resp)
		}
	})
	err = server.ListenAndServe()
	if err != nil {
		fmt.Print(err)
	}
}

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
}

type error struct {
	Error string `json:"error"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respBody := error{Error: msg}
	resp, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(code)
	w.Write(resp)
}

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	resp, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(code)
	w.Write(resp)
}

func cleanChirp(s string) string {
	profanities := []string{"kerfuffle", "sharbert", "fornax"}
	words := strings.Split(s, " ")
	for _, profanity := range profanities {
		for i := range words {
			if strings.ToLower(words[i]) == profanity {
				words[i] = "****"
			}
		}
	}
	cleanS := strings.Join(words, " ")
	return cleanS
}
