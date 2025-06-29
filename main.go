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
	"time"

	"github.com/google/uuid"
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
		if platform := os.Getenv("PLATFORM"); platform != "dev" {
			w.WriteHeader(403)
		} else {
			apiCfg.fileserverHits.Store(0)
			_ = apiCfg.dbQueries.DeleteAllUsers(req.Context())
			_ = apiCfg.dbQueries.DeleteAllChirps(req.Context())
			w.WriteHeader(200)
		}
	})
	serveMux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, req *http.Request) {
		newChirp := Chirp{}
		decoder := json.NewDecoder(req.Body)
		err := decoder.Decode(&newChirp)
		if err != nil {
			respondWithError(w, 400, "Error decoding chirp")
			return
		}
		if len(newChirp.Body) > 140 {
			respondWithError(w, 400, "Chirp is too long")
			return
		}
		newChirp.Body = cleanChirp(newChirp.Body)
		c, err := apiCfg.dbQueries.CreateChirp(req.Context(), database.CreateChirpParams{Body: newChirp.Body, UserID: newChirp.UserID})
		if err != nil {
			respondWithError(w, 500, err.Error())
			return
		}
		newChirp.ID = c.ID
		newChirp.CreatedAt = c.CreatedAt
		newChirp.UpdatedAt = c.UpdatedAt
		respondWithJSON(w, 201, newChirp)
	})
	serveMux.HandleFunc("GET /api/chirps", func(w http.ResponseWriter, req *http.Request) {
		dbChirps, err := apiCfg.dbQueries.ListChirps(req.Context())
		if err != nil {
			respondWithError(w, 500, err.Error())
			return
		}
		chirps := make([]Chirp, len(dbChirps))
		for i := range dbChirps {
			chirps[i] = Chirp(dbChirps[i])
		}
		respondWithJSON(w, 200, chirps)
	})
	serveMux.HandleFunc("GET /api/chirps/{chirpID}", func(w http.ResponseWriter, req *http.Request) {
		chirpIDstring := req.PathValue("chirpID")
		chirpID, err := uuid.Parse(chirpIDstring)
		if err != nil {
			respondWithError(w, 400, "Invalid ChirpID")
		}
		dbChirp, err := apiCfg.dbQueries.GetChirp(req.Context(), chirpID)
		if err != nil {
			respondWithError(w, 404, "Chirp doesn't exist")
		}
		chirp := Chirp(dbChirp)
		respondWithJSON(w, 200, chirp)
	})
	serveMux.HandleFunc("POST /api/users", func(w http.ResponseWriter, req *http.Request) {
		newUser := User{}
		decoder := json.NewDecoder(req.Body)
		err := decoder.Decode(&newUser)
		if err != nil {
			respondWithError(w, 400, "Error decoding user")
			return
		}
		u, err := apiCfg.dbQueries.CreateUser(req.Context(), newUser.Email)
		if err != nil {
			respondWithError(w, 500, err.Error())
			return
		}
		newUser.ID = u.ID
		newUser.CreatedAt = u.CreatedAt
		newUser.UpdatedAt = u.UpdatedAt
		respondWithJSON(w, 201, newUser)
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

type Chirp struct {
	ID        uuid.UUID     `json:"id"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Body      string        `json:"body"`
	UserID    uuid.NullUUID `json:"user_id"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
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
