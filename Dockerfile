# Stage 1: Build the React frontend
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

# Stage 3: Build the Go binary
FROM golang:1.24-alpine AS backend-builder

# Build arguments for flexibility
ARG CGO_ENABLED=0
ARG GOAMD64=v1

# Set working directory
WORKDIR /build

# Install git (gcc/musl-dev not needed for CGO_ENABLED=0)
RUN apk add --no-cache git

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY cmd/ cmd/
COPY config/ config/
COPY internal/ internal/
COPY pkg/ pkg/

# Build the application with maximum CPU compatibility
# CGO_ENABLED=0: Pure Go static binary (no C dependencies)
# GOAMD64=v1: Baseline x86-64 instruction set (compatible with all x86-64 CPUs from 2003+)
# -ldflags="-w -s": Strip debug info for smaller binary
RUN CGO_ENABLED=${CGO_ENABLED} GOOS=linux GOAMD64=${GOAMD64} go build \
    -ldflags="-w -s" \
    -o /tmp/server \
    ./cmd/api

# Stage 4: Create the runtime container
FROM alpine:latest

# Add necessary runtime packages
RUN apk add --no-cache ca-certificates tzdata postgresql-client

# Create application directory structure
WORKDIR /app
RUN mkdir -p /app/console/dist /app/notification_center/dist /app/data

# Copy the binary from the builder stage
COPY --from=backend-builder /tmp/server /app/server

# Copy the built console files
COPY --from=console-frontend-builder /build/console/dist/ /app/console/dist/

# Copy the built notification center files
COPY --from=notification-center-builder /build/notification_center/dist/ /app/notification_center/dist/

# Expose the application port
EXPOSE 8080

# Run the application
CMD ["/app/server"] 