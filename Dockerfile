# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /webhook-gateway ./cmd/server

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /

COPY --from=builder /webhook-gateway /webhook-gateway

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/webhook-gateway"]
