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
	"slices"
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
	apiCfg.polkaKey = os.Getenv("POLKA_KEY")
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
			return
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
		var dbChirps []database.Chirp
		author := req.URL.Query().Get("author_id")
		if author == "" {
			dbChirps, err = apiCfg.dbQueries.ListChirps(req.Context())
			if err != nil {
				respondWithError(w, 500, err.Error())
				return
			}
		} else {
			userID, err := uuid.Parse(author)
			if err != nil {
				respondWithError(w, 400, "User not found")
				return
			}
			dbChirps, err = apiCfg.dbQueries.ListChirpsFromAuthor(req.Context(), userID)
			if err != nil {
				respondWithError(w, 500, err.Error())
				return
			}
		}
		chirps := make([]Chirp, len(dbChirps))
		for i := range dbChirps {
			chirps[i] = Chirp(dbChirps[i])
		}
		sort := req.URL.Query().Get("sort")
		switch sort {
		case "", "asc":
			respondWithJSON(w, 200, chirps)
		case "desc":
			slices.Reverse(chirps)
			respondWithJSON(w, 200, chirps)
		}
	})
	serveMux.HandleFunc("GET /api/chirps/{chirpID}", func(w http.ResponseWriter, req *http.Request) {
		chirpIDstring := req.PathValue("chirpID")
		chirpID, err := uuid.Parse(chirpIDstring)
		if err != nil {
			respondWithError(w, 400, "Invalid ChirpID")
			return
		}
		dbChirp, err := apiCfg.dbQueries.GetChirp(req.Context(), chirpID)
		if err != nil {
			respondWithError(w, 404, "Chirp doesn't exist")
			return
		}
		chirp := Chirp(dbChirp)
		respondWithJSON(w, 200, chirp)
	})
	serveMux.HandleFunc("POST /api/users", func(w http.ResponseWriter, req *http.Request) {
		userCreds := UserCreds{}
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
		newUser.IsChirpyRed = u.IsChirpyRed.Bool
		respondWithJSON(w, 201, newUser)
	})
	serveMux.HandleFunc("POST /api/login", func(w http.ResponseWriter, req *http.Request) {
		userCreds := UserCreds{}
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
		token, err := auth.MakeJWT(thisUser.ID, apiCfg.secret)
		if err != nil {
			respondWithError(w, 500, "Error generating JWT token")
			return
		}
		refreshToken, _ := auth.MakeRefreshToken()
		err = apiCfg.dbQueries.CreateRefreshToken(req.Context(), database.CreateRefreshTokenParams{Token: refreshToken, UserID: thisUser.ID, ExpiresAt: time.Now().Add(time.Hour * 24 * 60)})
		if err != nil {
			respondWithError(w, 500, err.Error())
			return
		}
		respondWithJSON(w, 200, User{
			ID:           thisUser.ID,
			CreatedAt:    thisUser.CreatedAt,
			UpdatedAt:    thisUser.UpdatedAt,
			Email:        thisUser.Email,
			IsChirpyRed:  thisUser.IsChirpyRed.Bool,
			Token:        token,
			RefreshToken: refreshToken,
		})
	})
	serveMux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, req *http.Request) {
		bearer, err := auth.GetBearerToken(req.Header)
		if err != nil {
			respondWithError(w, 400, "Error fetching authorization token")
			return
		}
		refreshToken, err := apiCfg.dbQueries.GetUserFromRefreshToken(req.Context(), bearer)
		if err != nil {
			respondWithError(w, 401, "Refresh token not found")
			return
		}
		if refreshToken.ExpiresAt.Compare(time.Now()) == -1 {
			respondWithError(w, 401, "Refresh token expired")
			return
		}
		if refreshToken.RevokedAt.Valid {
			respondWithError(w, 401, "Refresh token revoked")
			return
		}
		accessToken, err := auth.MakeJWT(refreshToken.UserID, apiCfg.secret)
		if err != nil {
			respondWithError(w, 500, "Error creating access token")
			return
		}
		respondWithJSON(w, 200, Token{Token: accessToken})
	})
	serveMux.HandleFunc("POST /api/revoke", func(w http.ResponseWriter, req *http.Request) {
		bearerToken, err := auth.GetBearerToken(req.Header)
		if err != nil {
			respondWithError(w, 400, "Error fetching authorization token")
			return
		}
		err = apiCfg.dbQueries.RevokeToken(req.Context(), bearerToken)
		if err != nil {
			respondWithError(w, 500, "Error revoking refresh token")
			return
		}
		respondWithJSON(w, 204, nil)
	})
	serveMux.HandleFunc("PUT /api/users", func(w http.ResponseWriter, req *http.Request) {
		bearerToken, err := auth.GetBearerToken(req.Header)
		if err != nil {
			respondWithError(w, 401, "Error fetching authorization token")
			return
		}
		userID, err := auth.ValidateJWT(bearerToken, apiCfg.secret)
		if err != nil {
			respondWithError(w, 401, "Invalid authorization token")
			return
		}
		userCreds := UserCreds{}
		decoder := json.NewDecoder(req.Body)
		err = decoder.Decode(&userCreds)
		if err != nil {
			respondWithError(w, 400, "Error decoding user data")
			return
		}
		hashedPassword, err := auth.HashPassword(userCreds.Password)
		if err != nil {
			respondWithError(w, 500, "Error hashing password")
			return
		}
		thisUser, err := apiCfg.dbQueries.ChangeEmailAndPassword(req.Context(), database.ChangeEmailAndPasswordParams{ID: userID, Email: userCreds.Email, HashedPassword: hashedPassword})
		if err != nil {
			respondWithError(w, 500, "Error updating user data")
			return
		}
		respondWithJSON(w, 200, User{
			ID:          thisUser.ID,
			CreatedAt:   thisUser.CreatedAt,
			UpdatedAt:   thisUser.UpdatedAt,
			Email:       thisUser.Email,
			IsChirpyRed: thisUser.IsChirpyRed.Bool,
		})
	})
	serveMux.HandleFunc("DELETE /api/chirps/{chirpID}", func(w http.ResponseWriter, req *http.Request) {
		bearerToken, err := auth.GetBearerToken(req.Header)
		if err != nil {
			respondWithError(w, 401, "Error fetching authorization token")
			return
		}
		userID, err := auth.ValidateJWT(bearerToken, apiCfg.secret)
		if err != nil {
			respondWithError(w, 401, "Invalid authorization token")
			return
		}
		chirpIDstring := req.PathValue("chirpID")
		chirpID, err := uuid.Parse(chirpIDstring)
		if err != nil {
			respondWithError(w, 404, "Chirp not found")
			return
		}
		dbChirp, err := apiCfg.dbQueries.GetChirp(req.Context(), chirpID)
		if err != nil {
			respondWithError(w, 404, "Chirp doesn't exist")
			return
		}
		if userID != dbChirp.UserID {
			respondWithError(w, 403, "You can't delete someone else's chirp")
			return
		}
		err = apiCfg.dbQueries.DeleteChirp(req.Context(), chirpID)
		if err != nil {
			respondWithError(w, 500, "Error deleting chirp")
			return
		}
		respondWithJSON(w, 204, nil)
	})
	serveMux.HandleFunc("POST /api/polka/webhooks", func(w http.ResponseWriter, req *http.Request) {
		apiKey, err := auth.GetAPIKey(req.Header)
		if err != nil {
			respondWithError(w, 401, "Error fetching API key")
			return
		}
		if apiKey != apiCfg.polkaKey {
			respondWithError(w, 401, "Invalid API Key")
			return
		}
		webhook := Webhook{}
		decoder := json.NewDecoder(req.Body)
		err = decoder.Decode(&webhook)
		if err != nil {
			respondWithError(w, 400, "Error decoding request body")
			return
		}
		if webhook.Event != "user.upgraded" {
			respondWithJSON(w, 204, nil)
			return
		}
		err = apiCfg.dbQueries.UpgradeUserToRed(req.Context(), webhook.Data.UserID)
		if err != nil {
			respondWithError(w, 404, "User not found")
			return
		}
		respondWithJSON(w, 204, nil)
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
	polkaKey       string
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
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
	IsChirpyRed  bool      `json:"is_chirpy_red"`
}

type UserCreds struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type Token struct {
	Token string `json:"token"`
}

type Webhook struct {
	Event string `json:"event"`
	Data  struct {
		UserID uuid.UUID `json:"user_id"`
	} `json:"data"`
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
