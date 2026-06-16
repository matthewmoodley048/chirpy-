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
	"github.com/matthewmoodley048/chirpy/internal/auth"
	"github.com/matthewmoodley048/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
	platform       string
	auth           string
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
type authParm struct {
	Authorization string `json:"Authorization"`
}
type userReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userRsp struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
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

func expErrJSONResp(code int, w http.ResponseWriter, customErrMsg string) {
	respBody := errResp{
		Error: customErrMsg,
	}

	dat, err := json.Marshal(respBody)
	if err != nil {
		errJSONResp(err, 500, w)
		return
	}

	writeJSONResp(dat, code, w)
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
	params := userReq{}

	err := decoder.Decode(&params)
	if err != nil {
		expErrJSONResp(400, w, fmt.Sprintf("%v", err))
		return
	}

	hashedPassword, e := auth.HashPassword(params.Password)

	if e != nil {
		expErrJSONResp(400, w, fmt.Sprintf("%v", e))
		return
	}

	arg := database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
	}

	rsp, err := cfg.queries.CreateUser(r.Context(), arg)
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

func (cfg *apiConfig) handlerUpdateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "invalid method", http.StatusBadRequest)
		return
	}
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errJSONResp(err, 401, w)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.auth)
	if err != nil {
		errJSONResp(err, 401, w)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := userReq{}

	err = decoder.Decode(&params)
	if err != nil {
		expErrJSONResp(400, w, fmt.Sprintf("%v", err))
		return
	}

	hashedPassword, e := auth.HashPassword(params.Password)

	if e != nil {
		expErrJSONResp(400, w, fmt.Sprintf("%v", e))
		return
	}

	arg := database.UpdateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
		ID:             userID,
	}

	rsp, err := cfg.queries.UpdateUser(r.Context(), arg)
	if err != nil {
		http.Error(w, "failed to create user", 500)
	}

	updatedUser := User{
		ID:        rsp.ID,
		CreatedAt: rsp.CreatedAt,
		UpdatedAt: rsp.UpdatedAt,
		Email:     rsp.Email,
	}

	dat, err := json.Marshal(updatedUser)
	if err != nil {
		errJSONResp(err, 500, w)
		return
	}

	writeJSONResp(dat, http.StatusOK, w)
}

func (cfg *apiConfig) handlerLoginUser(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		http.Error(w, "invalid method", http.StatusBadRequest)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := userReq{}
	expiry := time.Hour

	err := decoder.Decode(&params)
	if err != nil {
		expErrJSONResp(400, w, fmt.Sprintf("%v", err))
		return
	}

	dbResult, err := cfg.queries.FetchUser(r.Context(), params.Email)

	if err == sql.ErrNoRows {
		errJSONResp(err, http.StatusUnauthorized, w)
	} else if err != nil {
		errJSONResp(err, 500, w)
	}

	validMatch, e := auth.CheckPasswordHash(params.Password, dbResult.HashedPassword)

	if e != nil {
		expErrJSONResp(400, w, fmt.Sprintf("%v", e))
		return
	}

	if !validMatch {
		errJSONResp(err, http.StatusUnauthorized, w)
		return
	}

	newToken, err := auth.MakeJWT(dbResult.ID, cfg.auth, expiry)
	if err != nil {
		errJSONResp(err, 500, w)
	}

	arg := database.CreateRefreshTokenParams{
		ExpiresAt: time.Now().Add(60 * 24 * time.Hour),
		UserID:    dbResult.ID,
		Token:     auth.MakeRefreshToken(),
	}
	newRefreshToken, err := cfg.queries.CreateRefreshToken(r.Context(), arg)
	if err != nil {
		errJSONResp(err, 500, w)
	}

	validUser := userRsp{
		ID:           dbResult.ID,
		CreatedAt:    dbResult.CreatedAt,
		UpdatedAt:    dbResult.UpdatedAt,
		Email:        dbResult.Email,
		Token:        newToken,
		RefreshToken: newRefreshToken.Token,
	}

	dat, err := json.Marshal(validUser)
	if err != nil {
		errJSONResp(err, 500, w)
		return
	}

	writeJSONResp(dat, http.StatusOK, w)
}

func (cfg *apiConfig) handlerFetchRefreshToken(w http.ResponseWriter, r *http.Request) {
	if r.Method == "" || r.Method == http.MethodGet {
		http.Error(w, "invalid method", http.StatusBadRequest)
		return
	}
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errJSONResp(err, 400, w)
		return
	}

	dbToken, err := cfg.queries.FetchRefreshToken(r.Context(), refreshToken)
	if err == sql.ErrNoRows {
		expErrJSONResp(http.StatusUnauthorized, w, "refresh token not found")
		return
	}

	if dbToken.RevokedAt.Valid {
		expErrJSONResp(http.StatusUnauthorized, w, "session expired")
		return
	}

	if dbToken.ExpiresAt.Before(time.Now().UTC()) {
		expErrJSONResp(http.StatusUnauthorized, w, "session expired")
		return
	}

	newToken, err := auth.MakeJWT(dbToken.UserID, cfg.auth, time.Hour)
	if err != nil {
		errJSONResp(err, 500, w)
	}

	validToken := userRsp{
		Token: newToken,
	}

	dat, err := json.Marshal(validToken)
	if err != nil {
		errJSONResp(err, 500, w)
		return
	}

	writeJSONResp(dat, http.StatusOK, w)
}

func (cfg *apiConfig) handlerRevokeRefreshToken(w http.ResponseWriter, r *http.Request) {
	if r.Method == "" || r.Method == http.MethodGet {
		http.Error(w, "invalid method", http.StatusBadRequest)
		return
	}
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errJSONResp(err, 400, w)
		return
	}

	dbToken, err := cfg.queries.FetchRefreshToken(r.Context(), refreshToken)
	if err == sql.ErrNoRows {
		expErrJSONResp(http.StatusUnauthorized, w, "refresh token not found")
		return
	}

	if dbToken.RevokedAt.Valid {
		expErrJSONResp(204, w, "session expired")
		return
	}

	err = cfg.queries.RevokeToken(r.Context(), dbToken.Token)
	if err != nil {
		expErrJSONResp(500, w, fmt.Sprintf("%v", err))
		return
	}
	writeJSONResp([]byte{}, 204, w)
}

func (cfg *apiConfig) handlerCreateChirp(w http.ResponseWriter, r *http.Request) {
	if r.Method == "" || r.Method == http.MethodGet {
		http.Error(w, "invalid method", http.StatusBadRequest)
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		expErrJSONResp(400, w, fmt.Sprintf("%v", err))
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		expErrJSONResp(http.StatusUnauthorized, w, fmt.Sprintf("%v", err))
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.auth)
	if err != nil {
		expErrJSONResp(http.StatusUnauthorized, w, fmt.Sprintf("%v", err))
		return
	}
	if len(params.Body) > 140 {
		expErrJSONResp(400, w, "Chirp is too long")
		return
	}

	if len(userID) <= 0 {
		expErrJSONResp(400, w, "No user specified")
		return
	}

	cleanBody := profanityFilter(params)

	createParams := database.CreateChirpParams{
		Body:   cleanBody.Cleaned_Body,
		UserID: userID,
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

func (cfg *apiConfig) handlerDeleteChirp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "invalid method", http.StatusBadRequest)
		return
	}
	authToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errJSONResp(err, 401, w)
		return
	}

	userID, err := auth.ValidateJWT(authToken, cfg.auth)
	if err != nil {
		errJSONResp(err, 401, w)
		return
	}

	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		errJSONResp(err, http.StatusBadRequest, w)
	}
	chirp, err := cfg.queries.GetChirp(r.Context(), chirpID)
	if err == sql.ErrNoRows {
		expErrJSONResp(404, w, "chirp not found")
		return
	}
	if err != nil {
		errJSONResp(err, 500, w)
		return
	}

	if chirp.UserID != userID {
		errJSONResp(err, 403, w)
		return

	}

	err = cfg.queries.DeleteChirp(r.Context(), chirpID)
	if err != nil {
		errJSONResp(err, 404, w)
		return
	}
	writeJSONResp([]byte{}, 204, w)
}

func (cfg *apiConfig) handlerGetChirp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "invalid method", http.StatusBadRequest)
		return
	}
	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		errJSONResp(err, http.StatusBadRequest, w)
	}
	chirp, e := cfg.queries.GetChirp(r.Context(), chirpID)
	if e == sql.ErrNoRows {
		expErrJSONResp(404, w, "chirp not found")
		return
	}
	if e != nil {
		errJSONResp(e, 500, w)
		return
	}

	body := Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}

	rsp, err := json.Marshal(body)
	if err != nil {
		errJSONResp(err, 500, w)
		return
	}

	writeJSONResp(rsp, http.StatusOK, w)
}

func (cfg *apiConfig) handlerGetAllChirps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "invalid method", http.StatusBadRequest)
		return
	}

	data, e := cfg.queries.GetAllChirps(r.Context())
	if e != nil {
		errJSONResp(e, 500, w)
		return
	}

	chirps := []Chirp{}

	for _, chirp := range data {
		chirps = append(chirps, Chirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		})
	}

	rsp, err := json.Marshal(chirps)
	if err != nil {
		errJSONResp(err, 500, w)
		return
	}

	writeJSONResp(rsp, http.StatusOK, w)
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")
	auth := os.Getenv("JWT")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		_ = fmt.Errorf("%v", err)
	}

	dbQueries := database.New(db)

	apiCfg := &apiConfig{queries: dbQueries, platform: platform, auth: auth}
	mux := http.NewServeMux()

	stripedRoot := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(stripedRoot))

	mux.HandleFunc("POST /api/chirps", apiCfg.handlerCreateChirp)
	mux.HandleFunc("GET /api/chirps", apiCfg.handlerGetAllChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerGetChirp)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.handlerDeleteChirp)
	mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)
	mux.HandleFunc("PUT /api/users", apiCfg.handlerUpdateUser)
	mux.HandleFunc("POST /api/login", apiCfg.handlerLoginUser)
	mux.HandleFunc("POST /api/refresh", apiCfg.handlerFetchRefreshToken)
	mux.HandleFunc("POST /api/revoke", apiCfg.handlerRevokeRefreshToken)

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
