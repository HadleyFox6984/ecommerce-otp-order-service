package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/infrai-examples/ecommerce-otp-order-service/internal/infrai"
	"github.com/infrai-examples/ecommerce-otp-order-service/internal/orders"
)

type server struct {
	workflow *orders.LoginWorkflow
}

type codeRequest struct {
	Phone     string `json:"phone"`
	RequestID string `json:"request_id"`
}

type verifyRequest struct {
	Phone      string `json:"phone"`
	Code       string `json:"code"`
	CustomerID string `json:"customer_id"`
	OrderID    string `json:"order_id"`
}

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}

	s := &server{workflow: orders.NewLoginWorkflow(infrai.NewSMSClient(apiKey))}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/code", s.requestCode)
	mux.HandleFunc("POST /login/verify", s.verifyCode)

	httpServer := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("order login service listening on %s", httpServer.Addr)
	log.Fatal(httpServer.ListenAndServe())
}

func (s *server) requestCode(w http.ResponseWriter, r *http.Request) {
	var input codeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.workflow.RequestCode(r.Context(), input.Phone, input.RequestID); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "code_sent", "request_id": input.RequestID})
}

func (s *server) verifyCode(w http.ResponseWriter, r *http.Request) {
	var input verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	result, err := s.workflow.VerifyAndLoadOrder(r.Context(), input.Phone, input.Code, input.CustomerID, input.OrderID)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, orders.ErrInvalidCode) {
			status = http.StatusUnauthorized
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
