# Stage 1: Build frontend
FROM node:20-alpine AS frontend-builder

WORKDIR /app/frontend

COPY frontend/package*.json ./
RUN npm ci --prefer-offline

COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go backend
FROM golang:1.24-alpine AS backend-builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app/pocketbase

COPY pocketbase/go.mod pocketbase/go.sum ./
RUN go mod download

COPY pocketbase/ ./

# Build arguments for version info
ARG APP_VERSION=dev
ARG BUILD_COMMIT=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=${APP_VERSION} -X main.Commit=${BUILD_COMMIT}" \
    -o /bin/autostack-server ./...

# Stage 3: Final minimal image
FROM gcr.io/distroless/static-debian12:nonroot

# Copy timezone data for scheduled jobs
COPY --from=backend-builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the compiled binary
COPY --from=backend-builder /bin/autostack-server /autostack-server

# Copy built frontend assets
COPY --from=frontend-builder /app/frontend/build /pb_public

# Copy migrations
COPY --from=backend-builder /app/pocketbase/pb_migrations /pb_migrations

# PocketBase data directory — mount a persistent volume here in production
VOLUME ["/pb_data"]

EXPOSE 8090

ENTRYPOINT ["/autostack-server"]
CMD ["serve", "--http=0.0.0.0:8090", "--publicDir=/pb_public"]
