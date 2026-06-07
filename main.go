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

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/matthewmoodley048/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
}

type parameters struct {
	Body string `json:"body"`
}

type errResp struct {
	Error string `json:"error"`
}

type validResp struct {
	Valid bool `json:"valid"`
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

	fltRsp := cleanResp{
		filteredSentence,
	}
	return fltRsp
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
	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
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

	//respBody := validResp{
	//Valid: true,
	//}

	respBody := profanityFilter(params)

	dat, err := json.Marshal(respBody)
	if err != nil {
		errJSONResp(err, 500, w)
		return
	}

	writeJSONResp(dat, http.StatusOK, w)
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)

	dbQueries := database.New(db)

	apiCfg := &apiConfig{queries: dbQueries}
	mux := http.NewServeMux()

	stripedRoot := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(stripedRoot))

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/healthz" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		body := "OK"
		w.Write([]byte(body))
	})
	mux.HandleFunc("POST /api/validate_chirp", apiCfg.handlerValidate)

	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)

	s := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	err = s.ListenAndServe()
	if err != nil {
		fmt.Errorf("%v", err)
	}
}
