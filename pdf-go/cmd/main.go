package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	chromedp "altinn/pdf/internal/chromedp"
	"altinn/pdf/internal/fetcher"
	"altinn/pdf/internal/types"
)

var generator types.PdfGenerator

func main() {
	outputPath := "./output/"
	chromePath, err := fetcher.Fetch(outputPath)
	if err != nil {
		log.Fatalf("Failed to fetch Chrome: %v", err)
	}

	generator, err = chromedp.New(chromePath)
	if err != nil {
		log.Fatalf("Failed to create PDF generator: %v", err)
	}
	defer generator.Close()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})
	http.HandleFunc("/pdf", handlePdfGeneration)

	fmt.Println("PDF server starting on :5011")
	log.Fatal(http.ListenAndServe(":5011", nil))
}

func handlePdfGeneration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req types.PdfRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	// Validate supported options
	if req.Options.Format != "" && req.Options.Format != "A4" {
		http.Error(w, "Only A4 format is supported", http.StatusBadRequest)
		return
	}

	// Ensure we have the authentication cookie
	var authToken string
	for _, cookie := range req.Cookies {
		if cookie.Name == "AltinnStudioRuntime" {
			authToken = cookie.Value
			break
		}
	}
	if authToken == "" {
		http.Error(w, "AltinnStudioRuntime cookie is required", http.StatusBadRequest)
		return
	}

	result, err := generator.Generate(context.Background(), req)
	if err != nil {
		log.Printf("Error generating PDF: %v", err)
		response := types.PdfResponse{Success: false, Error: err.Error()}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(result.Data)))
	w.Header().Set("X-Browser-Product", result.Browser.Product)
	w.Header().Set("X-Browser-Revision", result.Browser.Revision)
	w.Header().Set("X-Browser-ProtocolVersion", result.Browser.ProtocolVersion)
	w.Write(result.Data)
}
