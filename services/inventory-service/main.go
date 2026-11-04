package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"github.com/nauman/reliability-chaos-lab/pkg/chaos"
	"github.com/nauman/reliability-chaos-lab/pkg/telemetry"
)

func main() {
	port := flag.String("port", ":8082", "Inventory service port")
	flag.Parse()

	log.Println("[INFO] Starting Upstream Inventory Microservice...")

	injector := chaos.NewInjector()
	collector := telemetry.NewMetricsCollector()

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"inventory-service"}`))
	})

	mux.HandleFunc("/metrics", collector.ServeMetrics)

	// Runtime Chaos Configuration Endpoint
	mux.HandleFunc("/chaos/configure", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var cfg chaos.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		injector.SetConfig(cfg)
		log.Printf("[CHAOS] Updated chaos config: %+v\n", cfg)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "configured", "config": cfg})
	})

	// Business Endpoint
	mux.HandleFunc("/items/", func(w http.ResponseWriter, r *http.Request) {
		itemID := r.URL.Path[len("/items/"):]
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"item_id":   itemID,
			"in_stock":  true,
			"quantity":  42,
			"status":    "available",
		})
	})

	handler := telemetry.TracingMiddleware(collector, injector.Middleware(mux))

	log.Printf("[INFO] Inventory Service listening on %s\n", *port)
	if err := http.ListenAndServe(*port, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
