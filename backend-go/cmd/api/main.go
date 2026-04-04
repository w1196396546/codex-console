package main

import (
	"log"
	"net/http"

	internalhttp "github.com/dou-jiang/codex-console/backend-go/internal/http"
)

func main() {
	addr := ":8080"
	log.Printf("api listening on %s", addr)

	if err := http.ListenAndServe(addr, internalhttp.NewRouter(nil)); err != nil {
		log.Fatal(err)
	}
}
