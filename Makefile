.PHONY: build run dev clean test frontend

# Variables
APP_NAME    := audiobook-factory
GO          := go
NPM         := npm
AIR         := air

BUILD_DIR   := ./bin

# Build the Go server
build:
	$(GO) build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server

# Run the server directly
run: build
	$(BUILD_DIR)/$(APP_NAME)

# Development with hot-reload (requires `air`)
dev:
	$(AIR) -c .air.toml

# Build frontend
frontend:
	cd web && $(NPM) install && $(NPM) run build

# Build frontend in dev/watch mode
frontend-dev:
	cd web && $(NPM) install && $(NPM) run dev

# Run tests
test:
	$(GO) test ./...

# Clean
clean:
	rm -rf $(BUILD_DIR) ./data ./web/dist

# Dependencies
deps:
	$(GO) mod tidy
	cd web && $(NPM) install

# Docker
docker-build:
	docker build -t $(APP_NAME) .

docker-run:
	docker run -p 8080:8080 -v $(PWD)/data:/app/data -v $(PWD)/configs:/app/configs --env-file .env $(APP_NAME)

# Install development tools
dev-setup:
	$(GO) install github.com/air-verse/air@latest
