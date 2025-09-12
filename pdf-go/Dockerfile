# Build stage
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Copy go mod files first for better dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd

# Runtime stage
FROM docker.io/chromedp/headless-shell:141.0.7378.4

RUN apt-get update && apt-get install -y curl

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Create output directory and pre-download Chrome
RUN mkdir -p ./output

# Expose the port (Go service runs on 5011)
EXPOSE 5011

# Run the service
ENTRYPOINT ["./main"]