package main

import (
	"fmt"
	"net/http"
)

func main() {
	serveMux := http.NewServeMux()
	server := http.Server{Handler: serveMux}
	server.Addr = ":8080"
	serveMux.Handle("/app/", http.StripPrefix("/app", http.FileServer(http.Dir("app"))))
	serveMux.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})
	err := server.ListenAndServe()
	if err != nil {
		fmt.Print(err)
	}
}
