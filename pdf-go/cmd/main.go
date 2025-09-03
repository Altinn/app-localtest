package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	chromedp "altinn/pdf/internal/chromedp"
	custom "altinn/pdf/internal/custom"
	gorod "altinn/pdf/internal/gorod"
	"altinn/pdf/internal/types"
)

var generator types.PdfGenerator

func main() {
	configuredGenerator := os.Getenv("PDF_GENERATOR")
	if configuredGenerator == "" {
		configuredGenerator = "chromedp"
	}

	var err error
	switch configuredGenerator {
	case "gorod":
		generator, err = gorod.New()
		if err != nil {
			log.Fatalf("Failed to create PDF generator: %v", err)
		}
	case "custom":
		generator, err = custom.New()
		if err != nil {
			log.Fatalf("Failed to create PDF generator: %v", err)
		}
	case "chromedp":
		generator, err = chromedp.New()
		if err != nil {
			log.Fatalf("Failed to create PDF generator: %v", err)
		}
	default:
		log.Fatalf("Unknown PDF generator: %s. Supported generators: custom, gorod, chromedp", configuredGenerator)
	}
	defer generator.Close()

	log.Printf("Using PDF generator: %s", configuredGenerator)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})
	http.HandleFunc("/pdf", handlePdfGeneration)

	log.Println("PDF server starting on :5011")
	log.Fatal(http.ListenAndServe(":5011", nil))
}

func handlePdfGeneration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemDetails(w, ProblemDetails{
			Type:   "about:blank",
			Title:  "Method Not Allowed",
			Status: http.StatusMethodNotAllowed,
			Detail: "Only POST method is allowed for PDF generation",
		})
		return
	}

	var req types.PdfRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblemDetails(w, ProblemDetails{
			Type:   "about:blank",
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
			Detail: "Invalid JSON in request body",
		})
		return
	}

	if req.URL == "" {
		writeProblemDetails(w, ProblemDetails{
			Type:   "about:blank",
			Title:  "Bad Request",
			Status: http.StatusBadRequest,
			Detail: "URL field is required",
		})
		return
	}

	result, err := generator.Generate(r.Context(), req)
	if err != nil {
		log.Printf("Error generating PDF: %v", err)

		statusCode, problemDetails := mapErrorToProblemDetails(err)
		problemDetails.Status = statusCode
		writeProblemDetails(w, problemDetails)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(result.Data)))
	w.Header().Set("X-Browser-Product", result.Browser.Product)
	w.Header().Set("X-Browser-Revision", result.Browser.Revision)
	w.Header().Set("X-Browser-ProtocolVersion", result.Browser.ProtocolVersion)
	w.Write(result.Data)
}

type ProblemDetails struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

func writeProblemDetails(w http.ResponseWriter, problem ProblemDetails) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)
	json.NewEncoder(w).Encode(problem)
}

func mapErrorToProblemDetails(err error) (int, ProblemDetails) {
	var pdfErr *types.PDFError
	if errors.As(err, &pdfErr) {
		switch {
		case errors.Is(pdfErr, types.ErrQueueFull):
			return http.StatusTooManyRequests, ProblemDetails{
				Type:   "about:blank",
				Title:  "Too Many Requests",
				Detail: "PDF generator queue is full, please try again later",
			}
		case errors.Is(pdfErr, types.ErrTimeout):
			return http.StatusRequestTimeout, ProblemDetails{
				Type:   "about:blank",
				Title:  "Request Timeout",
				Detail: "PDF generation timed out during processing",
			}
		case errors.Is(pdfErr, types.ErrSetCookieFail):
			return http.StatusBadRequest, ProblemDetails{
				Type:   "about:blank",
				Title:  "Bad Request",
				Detail: "Failed to set cookies for PDF generation",
			}
		case errors.Is(pdfErr, types.ErrElementNotReady):
			return http.StatusBadRequest, ProblemDetails{
				Type:   "about:blank",
				Title:  "Bad Request",
				Detail: "Wait condition element not ready within timeout",
			}
		case errors.Is(pdfErr, types.ErrGenerationFail):
			return http.StatusInternalServerError, ProblemDetails{
				Type:   "about:blank",
				Title:  "Internal Server Error",
				Detail: "PDF generation failed",
			}
		case errors.Is(pdfErr, types.ErrUnhandledBrowserError):
			return http.StatusInternalServerError, ProblemDetails{
				Type:   "about:blank",
				Title:  "Internal Server Error",
				Detail: "Browser operation failed unexpectedly",
			}
		}
	}

	// Default error handling
	return http.StatusInternalServerError, ProblemDetails{
		Type:   "about:blank",
		Title:  "Internal Server Error",
		Detail: "An unexpected error occurred during PDF generation",
	}
}
