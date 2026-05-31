package main

import (
	"fmt"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	stripedRoot := http.StripPrefix("/app", http.FileServer(http.Dir(".")))
	mux.Handle("/app/", stripedRoot)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/healthz" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		body := "OK"
		w.Write([]byte(body))
	})

	s := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	err := s.ListenAndServe()
	if err != nil {
		fmt.Errorf("%v", err)
	}
}
