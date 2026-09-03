# Stage 1: Build the React frontend
FROM node:22-alpine AS console-frontend-builder

# Set working directory for the frontend
WORKDIR /build/console

# Copy frontend package files
COPY console/package*.json ./

# Install dependencies with retry settings for network resilience
RUN npm config set fetch-retries 5 && \
    npm config set fetch-retry-mintimeout 20000 && \
    npm config set fetch-retry-maxtimeout 120000 && \
    npm ci

# Copy frontend source code
COPY console/ ./

# Build frontend in production mode
RUN npm run build

# Stage 2: Build the notification center frontend
FROM node:22-alpine AS notification-center-builder

# Set working directory for the notification center
WORKDIR /build/notification_center

# Copy notification center package files
COPY notification_center/package*.json ./

# Install dependencies with retry settings for network resilience
RUN npm config set fetch-retries 5 && \
    npm config set fetch-retry-mintimeout 20000 && \
    npm config set fetch-retry-maxtimeout 120000 && \
    npm ci

# Copy notification center source code
COPY notification_center/ ./

# Build notification center in production mode
RUN npm run build

# Stage 2b: Build the web analytics browser SDK (embedded into the Go binary)
FROM node:22-alpine AS web-analytics-sdk-builder

WORKDIR /build/web_analytics_sdk

COPY web_analytics_sdk/package*.json ./

RUN npm config set fetch-retries 5 && \
    npm config set fetch-retry-mintimeout 20000 && \
    npm config set fetch-retry-maxtimeout 120000 && \
    npm ci

COPY web_analytics_sdk/ ./

# The SDK version is read from the Go config at build time (single source of
# truth for the release), so that file must be present in this stage too.
COPY config/config.go /build/config/config.go

RUN npm run build

# Stage 3: Build the Go binary (pure Go, no CGO needed)
FROM golang:1.25-alpine AS backend-builder

# Set working directory
WORKDIR /build

# Install build dependencies
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

# Build the application with CGO disabled (pure Go)
ENV CGO_ENABLED=0
ENV GOOS=linux
RUN go build -ldflags="-s -w" -o /tmp/server ./cmd/api

# Stage 4: Create the runtime container (Alpine for smaller image)
FROM alpine:3.19

# Add necessary runtime packages
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    postgresql-client

# Create application directory structure
WORKDIR /app
RUN mkdir -p /app/console/dist /app/notification_center/dist /app/web_analytics_sdk/dist /app/data /app/geoip

# Copy the binary from the builder stage
COPY --from=backend-builder /tmp/server /app/server

# Copy the built console files
COPY --from=console-frontend-builder /build/console/dist/ /app/console/dist/

# Copy the built notification center files
COPY --from=notification-center-builder /build/notification_center/dist/ /app/notification_center/dist/

# Ship the web analytics browser SDK as a static asset, not inside the binary.
# The bundle links ua-parser-js (AGPL-3.0-or-later), so embedding it would
# carry that code into the server binary itself; served from disk, the licence
# boundary stays at web_analytics_sdk/, which carries its own LICENSE and
# NOTICE. The server looks it up relative to its working directory (/app) and
# simply does not register /na.js if it is missing.
COPY --from=web-analytics-sdk-builder /build/web_analytics_sdk/dist/notifuse-analytics.min.js /app/web_analytics_sdk/dist/notifuse-analytics.min.js
COPY web_analytics_sdk/LICENSE /app/web_analytics_sdk/LICENSE
COPY web_analytics_sdk/NOTICE /app/web_analytics_sdk/NOTICE

# Ship the MaxMind GeoLite2 City database so web analytics resolves locations
# with no configuration. NOT under /app/data: compose bind-mounts the host's
# ./data over that directory, which would hide anything the image put there.
# A fresher database dropped in the mounted ./data takes precedence (see
# geoip.DefaultPaths), as does GEOIP_DB_PATH, neither needing a rebuild.
# This product includes GeoLite2 data created by MaxMind, available from
# https://www.maxmind.com.
COPY data/GeoLite2-City.mmdb /app/geoip/GeoLite2-City.mmdb

# Expose the application ports
EXPOSE 8080
EXPOSE 587

# Run the application
CMD ["/app/server"] 