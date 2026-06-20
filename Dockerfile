# Stage 1: Build Go server
FROM golang:1.23-bookworm AS go-builder
RUN apt-get update && apt-get install -y gcc libc-dev libsqlite3-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=1 go build -o /server ./cmd/server

# Stage 2: Build frontend
FROM node:20-alpine AS fe-builder
WORKDIR /app
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 3: Runtime
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    sqlite3 \
    ffmpeg \
    nodejs \
    npm \
    python3 \
    python3-pip \
    calibre \
    poppler-utils \
    unzip \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Install epub2md globally
RUN npm install -g epub2md

# Install opendataloader-pdf (Python)
RUN pip3 install --break-system-packages opendataloader-pdf

WORKDIR /app

# Copy Go server
COPY --from=go-builder /server /app/server

# Copy configs
COPY configs/ /app/configs/

# Copy frontend dist
COPY --from=fe-builder /app/dist /app/web/dist

# Create data directories
RUN mkdir -p /app/data/uploads /app/data/output

EXPOSE 8080

ENV CONFIG_PATH=/app/configs

CMD ["/app/server"]
