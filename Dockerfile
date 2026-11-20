# Tahap 1: Build Aplikasi
FROM golang:1.27-alpine AS builder
WORKDIR /app

# Copy dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy semua source code
COPY . .

# Build (karena main.go sudah di root, pakai titik)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server .

# Tahap 2: Jalankan Aplikasi
FROM alpine:3.21
WORKDIR /app
COPY --from=builder /app/server .

# Buka port (Delcom pakai 7243 sesuai .env Anda)
EXPOSE 7243

# Jalankan
CMD ["./server"]