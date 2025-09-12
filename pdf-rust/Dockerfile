# Build stage
FROM rust:1.89-bookworm AS builder

WORKDIR /app

# Copy dependency files first for better caching
COPY Cargo.toml Cargo.lock ./

# Create a dummy main.rs to build dependencies
RUN mkdir src && \
    echo "fn main() {}" > src/main.rs && \
    cargo build --release && \
    rm -rf src

# Copy source code
COPY src/ src/

# Build the actual application
RUN cargo build --release

# Runtime stage
FROM debian:bookworm-slim

# Install Chrome dependencies and clean up in one layer
RUN apt-get update && \
    apt-get install -y \
        ca-certificates \
        fonts-liberation \
        libappindicator3-1 \
        libasound2 \
        libatk-bridge2.0-0 \
        libatk1.0-0 \
        libcups2 \
        libdbus-1-3 \
        libdrm2 \
        libgbm1 \
        libgtk-3-0 \
        libnspr4 \
        libnss3 \
        libxcomposite1 \
        libxdamage1 \
        libxfixes3 \
        libxrandr2 \
        libxss1 \
        lsb-release \
        wget \
        xdg-utils \
        curl \
        unzip \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create a non-root user
RUN groupadd --gid 1001 appuser && \
    useradd --uid 1001 --gid appuser --shell /bin/bash --create-home appuser

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/target/release/pdf-rust .

# Create output directory and pre-download Chrome
RUN mkdir -p ./output && \
    cd ./output && \
    curl -L -o chrome-linux64.zip https://storage.googleapis.com/chrome-for-testing-public/117.0.5938.92/linux64/chrome-linux64.zip && \
    unzip chrome-linux64.zip && \
    chmod +x chrome-linux64/chrome && \
    chmod 4755 chrome-linux64/chrome_sandbox && \
    rm chrome-linux64.zip && \
    chown -R appuser:appuser /app && \
    chown root:root /app/output/chrome-linux64/chrome_sandbox

# Switch to non-root user
USER appuser

# Expose the port
EXPOSE 5010

# Run the service with chromiumoxide implementation
CMD ["./pdf-rust", "chromiumoxide"]