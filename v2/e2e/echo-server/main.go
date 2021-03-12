package main

import (
	"io"
	"net/http"
)

type echoHandler struct{}

func (echoHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "application/octet-stream")
	w.Write(body)
}

func main() {
	s := &http.Server{
		Handler: echoHandler{},
	}
	s.ListenAndServe()
}
