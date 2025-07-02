package main

import (
	"chirpy/internal/auth"
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
	apiCfg.secret = os.Getenv("JWT_SECRET")
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
		token, err := auth.GetBearerToken(req.Header)
		if err != nil {
			respondWithError(w, 400, "Error fetching authorization token")
			return
		}
		userID, err := auth.ValidateJWT(token, apiCfg.secret)
		if err != nil {
			respondWithError(w, 401, err.Error())
		}
		newChirp := Chirp{}
		newChirp.UserID = userID
		decoder := json.NewDecoder(req.Body)
		err = decoder.Decode(&newChirp)
		if err != nil {
			respondWithError(w, 400, "Error decoding chirp")
			return
		}
		if len(newChirp.Body) > 140 {
			respondWithError(w, 400, "Chirp is too long")
			return
		}
		newChirp.Body = cleanChirp(newChirp.Body)
		c, err := apiCfg.dbQueries.CreateChirp(req.Context(), database.CreateChirpParams{Body: newChirp.Body, UserID: userID})
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
		userCreds := Login{}
		decoder := json.NewDecoder(req.Body)
		err := decoder.Decode(&userCreds)
		if err != nil {
			respondWithError(w, 400, "Error decoding user data")
			return
		}
		hashedPassword, err := auth.HashPassword(userCreds.Password)
		if err != nil {
			respondWithError(w, 500, "Error hashing password")
			return
		}
		u, err := apiCfg.dbQueries.CreateUser(req.Context(), database.CreateUserParams{Email: userCreds.Email, HashedPassword: hashedPassword})
		if err != nil {
			respondWithError(w, 500, err.Error())
			return
		}
		newUser := User{}
		newUser.ID = u.ID
		newUser.CreatedAt = u.CreatedAt
		newUser.UpdatedAt = u.UpdatedAt
		newUser.Email = userCreds.Email
		respondWithJSON(w, 201, newUser)
	})
	serveMux.HandleFunc("POST /api/login", func(w http.ResponseWriter, req *http.Request) {
		userCreds := Login{}
		userCreds.ExpiresIn = time.Hour
		decoder := json.NewDecoder(req.Body)
		err := decoder.Decode(&userCreds)
		if err != nil {
			respondWithError(w, 400, "Error decoding user data")
			return
		}
		thisUser, err := apiCfg.dbQueries.GetUser(req.Context(), userCreds.Email)
		if err != nil {
			respondWithError(w, 401, "Incorrect email or password")
			return
		}
		if err = auth.CheckPasswordHash(userCreds.Password, thisUser.HashedPassword); err != nil {
			respondWithError(w, 401, "Incorrect email or password")
			return
		}
		token, err := auth.MakeJWT(thisUser.ID, apiCfg.secret, userCreds.ExpiresIn)
		if err != nil {
			respondWithError(w, 500, "Error generating JWT token")
		}
		respondWithJSON(w, 200, User{
			ID:        thisUser.ID,
			CreatedAt: thisUser.CreatedAt,
			UpdatedAt: thisUser.UpdatedAt,
			Email:     thisUser.Email,
			Token:     token,
		})
	})
	err = server.ListenAndServe()
	if err != nil {
		fmt.Print(err)
	}
}

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	secret         string
}

type error struct {
	Error string `json:"error"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
	Token     string    `json:"token"`
}

type Login struct {
	Email     string        `json:"email"`
	Password  string        `json:"password"`
	ExpiresIn time.Duration `json:"expires_in_seconds"`
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
