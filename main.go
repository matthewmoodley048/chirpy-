package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Hits: %v", cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
}

//func handlerHealth(w http.ResponseWriter, r *http.Request) {
//	if r.URL.Path != "/healthz" {
//		http.NotFound(w, r)
//		return
//	}
//	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
//	w.WriteHeader(http.StatusOK)
//	body := "OK"
//	w.Write([]byte(body))
//}

func main() {
	apiCfg := &apiConfig{}
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

	mux.HandleFunc("GET /api/metrics", apiCfg.handlerMetrics)
	mux.HandleFunc("POST /api/reset", apiCfg.handlerReset)

	s := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	err := s.ListenAndServe()
	if err != nil {
		fmt.Errorf("%v", err)
	}
}
