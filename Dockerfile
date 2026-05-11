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

# Stage 3: Final image
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates curl unzip && rm -rf /var/lib/apt/lists/*

# Install Terraform 1.9.5 (match what CI validates against)
RUN curl -fsSL https://releases.hashicorp.com/terraform/1.9.5/terraform_1.9.5_linux_amd64.zip -o terraform.zip \
    && unzip terraform.zip \
    && mv terraform /usr/local/bin/ \
    && rm terraform.zip \
    && terraform --version

# Copy the compiled binary
COPY --from=backend-builder /bin/autostack-server /autostack-server

# Copy built frontend assets
COPY --from=frontend-builder /app/frontend/build /pb_public

# Copy migrations
COPY --from=backend-builder /app/pocketbase/pb_migrations /pb_migrations

# Copy templates
COPY --from=backend-builder /app/pocketbase/templates /templates

# PocketBase data directory — mount a persistent volume here in production
VOLUME ["/pb_data"]

EXPOSE 8090

ENTRYPOINT ["/autostack-server"]
CMD ["serve", "--http=0.0.0.0:8090", "--publicDir=/pb_public"]
