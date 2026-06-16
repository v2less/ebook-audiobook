# Stage 1: Build Go server
FROM golang:1.23-alpine AS go-builder
RUN apk add --no-cache gcc musl-dev sqlite-dev
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
FROM alpine:3.20
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    sqlite \
    ffmpeg \
    nodejs \
    npm \
    python3 \
    py3-pip \
    calibre \
    poppler-utils \
    unzip \
    curl

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
