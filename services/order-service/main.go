package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/nauman/reliability-chaos-lab/pkg/circuitbreaker"
	"github.com/nauman/reliability-chaos-lab/pkg/telemetry"
)

type OrderService struct {
	inventoryURL string
	breaker      *circuitbreaker.Breaker
	client       *http.Client
	collector    *telemetry.MetricsCollector
}

func main() {
	port := flag.String("port", ":8081", "Order service port")
	inventoryURL := flag.String("inventory-url", "http://localhost:8082", "Inventory upstream service URL")
	flag.Parse()

	log.Println("[INFO] Starting Resilient Order Microservice...")

	collector := telemetry.NewMetricsCollector()
	// Circuit breaker: trip after 3 consecutive errors; 5 second recovery test window
	breaker := circuitbreaker.NewBreaker(3, 5*time.Second)

	svc := &OrderService{
		inventoryURL: *inventoryURL,
		breaker:      breaker,
		collector:    collector,
		client: &http.Client{
			Timeout: 1000 * time.Millisecond, // Strict timeout prevents thread exhaustion
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"order-service"}`))
	})
	mux.HandleFunc("/metrics", collector.ServeMetrics)

	mux.HandleFunc("/orders", svc.handleCreateOrder)

	handler := telemetry.TracingMiddleware(collector, mux)

	log.Printf("[INFO] Order Service listening on %s\n", *port)
	if err := http.ListenAndServe(*port, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func (s *OrderService) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		OrderID string `json:"order_id"`
		ItemID  string `json:"item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	corrID, _ := r.Context().Value(telemetry.CorrelationIDKey).(string)

	// Execute call to Inventory service protected by Circuit Breaker
	var inventoryResp map[string]any
	err := s.breaker.Execute(func() error {
		url := fmt.Sprintf("%s/items/%s", s.inventoryURL, req.ItemID)
		httpReq, _ := http.NewRequestWithContext(r.Context(), "GET", url, nil)
		httpReq.Header.Set("X-Correlation-ID", corrID)

		resp, err := s.client.Do(httpReq)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("upstream 5xx error: %s", string(body))
		}

		return json.NewDecoder(resp.Body).Decode(&inventoryResp)
	})

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
			s.collector.RecordBreakerTrip()
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":          "circuit_breaker_open",
				"message":        "Inventory service is degraded. Fast-failing to preserve order service availability.",
				"breaker_state":  s.breaker.State(),
				"correlation_id": corrID,
			})
			return
		}

		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":          "upstream_failure",
			"message":        err.Error(),
			"breaker_state":  s.breaker.State(),
			"correlation_id": corrID,
		})
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"order_id":       req.OrderID,
		"status":         "confirmed",
		"inventory":      inventoryResp,
		"breaker_state":  s.breaker.State(),
		"correlation_id": corrID,
	})
}
