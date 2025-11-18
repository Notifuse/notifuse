# Stage 1: Build the React frontend and bundle LiquidJS
FROM node:20-alpine AS console-frontend-builder

# Set working directory for the frontend
WORKDIR /build/console

# Copy frontend package files
COPY console/package*.json ./

# Install dependencies
RUN npm ci

# Copy frontend source code
COPY console/ ./

# Build frontend in production mode
RUN npm run build

# Bundle LiquidJS for Go backend (V8 + LiquidJS)
# Create the target directory and copy the bundle
RUN mkdir -p ../pkg/liquid && \
    cp node_modules/liquidjs/dist/liquid.browser.umd.js ../pkg/liquid/liquid.bundle.js

# Stage 2: Build the notification center frontend
FROM node:20-alpine AS notification-center-builder

# Set working directory for the notification center
WORKDIR /build/notification_center

# Copy notification center package files
COPY notification_center/package*.json ./

# Install dependencies
RUN npm ci

# Copy notification center source code
COPY notification_center/ ./

# Build notification center in production mode
RUN npm run build

# Stage 3: Build the Go binary (with CGO for V8)
FROM golang:1.25-bookworm AS backend-builder

# Set working directory
WORKDIR /build

# Install build dependencies for V8 (CGO)
RUN apt-get update && apt-get install -y \
    build-essential \
    git \
    && rm -rf /var/lib/apt/lists/*

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY cmd/ cmd/
COPY config/ config/
COPY internal/ internal/
COPY pkg/ pkg/

# Copy the bundled liquidjs from console build stage
COPY --from=console-frontend-builder /build/pkg/liquid/liquid.bundle.js /build/pkg/liquid/liquid.bundle.js

# Build the application with CGO enabled (required for V8)
ENV CGO_ENABLED=1
ENV GOOS=linux
RUN go build -o /tmp/server ./cmd/api

# Stage 4: Create the runtime container (Debian for V8 runtime libs)
FROM debian:bookworm-slim

# Add necessary runtime packages
RUN apt-get update && apt-get install -y \
    ca-certificates \
    tzdata \
    postgresql-client \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

# Create application directory structure
WORKDIR /app
RUN mkdir -p /app/console/dist /app/notification_center/dist /app/data

# Copy the binary from the builder stage
COPY --from=backend-builder /tmp/server /app/server

# Copy the built console files
COPY --from=console-frontend-builder /build/console/dist/ /app/console/dist/

# Copy the built notification center files
COPY --from=notification-center-builder /build/notification_center/dist/ /app/notification_center/dist/

# Expose the application ports
EXPOSE 8080
EXPOSE 587

# Run the application
CMD ["/app/server"] 