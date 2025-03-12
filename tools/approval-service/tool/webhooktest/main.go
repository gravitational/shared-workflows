package main

import (
	"embed"
	"log"
	"net/http"
)

//go:embed payload.json
var payloadFile embed.FS

func main() {

	client := &http.Client{}

	payload, err := payloadFile.Open("payload.json")
	if err != nil {
		log.Fatal(err)
	}

	req, err := http.NewRequest("POST", "http://localhost:8080/webhook", payload)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "deployment_review")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
}
