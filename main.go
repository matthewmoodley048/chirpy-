package main

import (
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
	"github.com/matthewmoodley048/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
	platform       string
}
type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type parameters struct {
	Body   string    `json:"body"`
	UserID uuid.UUID `json:"user_id"`
}

type emailReq struct {
	Email string `json:"email"`
}

type errResp struct {
	Error string `json:"error"`
}

type cleanResp struct {
	Cleaned_Body string `json:"cleaned_body"`
}

func profanityFilter(b parameters) cleanResp {
	msg := b.Body
	words := strings.Split(msg, " ")
	badWords := map[string]struct{}{"kerfuffle": {}, "sharbert": {}, "fornax": {}}
	for i, word := range words {
		if _, ok := badWords[strings.ToLower(word)]; ok {
			words[i] = "****"
		}
	}
	filteredSentence := strings.Join(words, " ")

	cleanRsp := cleanResp{
		filteredSentence,
	}
	return cleanRsp
}

func writeJSONResp(dat []byte, code int, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(dat)
}

func errJSONResp(err error, code int, w http.ResponseWriter) {
	log.Printf("Error marshalling JSON: %s", err)
	w.WriteHeader(code)
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	rsp, e := w.Write([]byte(fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1>    <p>Chirpy has been visited %d times!</p>  </body></html>", cfg.fileserverHits.Load())))
	if e != nil {
		errJSONResp(e, rsp, w)
		return
	}
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.WriteHeader(403)
	}
	cfg.fileserverHits.Store(0)
	err := cfg.queries.DeleteAllUsers(r.Context())
	if err != nil {
		errJSONResp(err, 500, w)
	}
	w.WriteHeader(http.StatusOK)
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method == "" || r.Method == http.MethodGet {
		http.Error(w, "invalid method", http.StatusBadRequest)
		return
	}
	decoder := json.NewDecoder(r.Body)
	params := emailReq{}

	err := decoder.Decode(&params)
	if err != nil {
		respBody := errResp{
			Error: fmt.Sprintf("%v", err),
		}

		dat, err := json.Marshal(respBody)
		if err != nil {
			errJSONResp(err, 500, w)
			return
		}

		writeJSONResp(dat, 400, w)
		return
	}

	rsp, err := cfg.queries.CreateUser(r.Context(), params.Email)
	if err != nil {
		http.Error(w, "failed to create user", 500)
	}

	createdUser := User{
		ID:        rsp.ID,
		CreatedAt: rsp.CreatedAt,
		UpdatedAt: rsp.UpdatedAt,
		Email:     rsp.Email,
	}

	dat, err := json.Marshal(createdUser)
	if err != nil {
		errJSONResp(err, 500, w)
		return
	}

	writeJSONResp(dat, 201, w)
}

func (cfg *apiConfig) handlerValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method == "" || r.Method == http.MethodGet {
		http.Error(w, "invalid method", http.StatusBadRequest)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		respBody := errResp{
			Error: fmt.Sprintf("%v", err),
		}

		dat, err := json.Marshal(respBody)
		if err != nil {
			errJSONResp(err, 500, w)
			return
		}

		writeJSONResp(dat, 400, w)
		return
	}

	if len(params.Body) > 140 {
		respBody := errResp{
			Error: "Chirp is too long",
		}

		dat, err := json.Marshal(respBody)
		if err != nil {
			errJSONResp(err, 500, w)
			return
		}

		writeJSONResp(dat, 400, w)
		return
	}

	if len(params.UserID) <= 0 {
		respBody := errResp{
			Error: "No user specified",
		}

		dat, err := json.Marshal(respBody)
		if err != nil {
			errJSONResp(err, 500, w)
			return
		}

		writeJSONResp(dat, 400, w)
		return
	} // respBody := validResp{
	//Valid: true,
	//}

	cleanBody := profanityFilter(params)

	createParams := database.CreateChirpParams{
		Body:   cleanBody.Cleaned_Body,
		UserID: params.UserID,
	}

	dbRsp, e := cfg.queries.CreateChirp(r.Context(), createParams)
	if e != nil {
		errJSONResp(e, 500, w)
		return
	}
	created := Chirp{
		ID:        dbRsp.ID,
		CreatedAt: dbRsp.CreatedAt,
		UpdatedAt: dbRsp.UpdatedAt,
		Body:      dbRsp.Body,
		UserID:    dbRsp.UserID,
	}

	dat, err := json.Marshal(created)
	if err != nil {
		errJSONResp(err, 500, w)
		return
	}

	writeJSONResp(dat, http.StatusCreated, w)
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		_ = fmt.Errorf("%v", err)
	}

	dbQueries := database.New(db)

	apiCfg := &apiConfig{queries: dbQueries, platform: platform}
	mux := http.NewServeMux()

	stripedRoot := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(stripedRoot))

	mux.HandleFunc("POST /api/chirps", apiCfg.handlerValidate)
	mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/healthz" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		body := "OK"
		_, _ = w.Write([]byte(body))
	})

	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)

	s := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	err = s.ListenAndServe()
	if err != nil {
		_ = fmt.Errorf("%v", err)
	}
}
