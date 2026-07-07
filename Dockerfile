# Stage 1: Build Go server
FROM golang:1.23-bookworm AS go-builder
# 使用清华镜像源加速 apt
RUN sed -i 's|http://deb.debian.org|http://mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources
RUN apt-get update && apt-get install -y gcc libc-dev libsqlite3-dev
# 使用国内镜像加速 Go 模块下载
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=1 go build -o /server ./cmd/server

# Stage 2: Build frontend
FROM node:20-alpine AS fe-builder
# 使用清华镜像源加速 npm
RUN npm config set registry https://registry.npmmirror.com
WORKDIR /app
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 3: Runtime
FROM debian:bookworm-slim
# 使用清华镜像源加速各包管理器
RUN sed -i 's|http://deb.debian.org|http://mirrors.tuna.tsinghua.edu.cn|g' /etc/apt/sources.list.d/debian.sources
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
    pandoc \
    poppler-utils \
    unzip \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Install epub2md globally
RUN npm config set registry https://registry.npmmirror.com && npm install -g epub2md

# Install pdf-inspector (PDF parsing with OCR detection)
RUN npm install -g @firecrawl/pdf-inspector

# Install opendataloader-pdf (Python)
RUN pip3 config set global.index-url https://pypi.tuna.tsinghua.edu.cn/simple && pip3 install --break-system-packages opendataloader-pdf

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
