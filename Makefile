.PHONY: all build test run-order run-inventory docker-up docker-down clean

BIN_DIR := ./bin

all: build test

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/order-service ./services/order-service
	go build -o $(BIN_DIR)/inventory-service ./services/inventory-service
	@echo "Build complete. Artifacts written to $(BIN_DIR)/"

test:
	go test -v -race ./pkg/...

run-order:
	go run ./services/order-service -port :8081 -inventory-url http://localhost:8082

run-inventory:
	go run ./services/inventory-service -port :8082

docker-up:
	docker compose up --build

docker-down:
	docker compose down

clean:
	rm -rf $(BIN_DIR)
