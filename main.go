package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

func main() {
	serveMux := http.NewServeMux()
	server := http.Server{Handler: serveMux}
	server.Addr = ":8080"
	apiCfg := &apiConfig{}
	apiCfg.fileserverHits.Store(0)
	serveMux.Handle("/app/", http.StripPrefix("/app", apiCfg.middlewareMetricsInc(http.FileServer(http.Dir("app")))))
	serveMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})
	serveMux.HandleFunc("GET /metrics", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		fmt.Fprintf(w, "Hits: %v", apiCfg.fileserverHits.Load())
	})
	serveMux.HandleFunc("POST /reset", func(w http.ResponseWriter, req *http.Request) {
		apiCfg.fileserverHits.Store(0)
	})
	err := server.ListenAndServe()
	if err != nil {
		fmt.Print(err)
	}
}

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}
