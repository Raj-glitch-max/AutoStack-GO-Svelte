FROM golang:latest AS builder

WORKDIR /app

# Install dependencies
RUN apt-get update && apt-get install -y ca-certificates curl unzip && rm -rf /var/lib/apt/lists/*

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .
RUN go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o autostack .

# Final stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates curl unzip && rm -rf /var/lib/apt/lists/*

# Install Terraform
RUN curl -fsSL https://releases.hashicorp.com/terraform/1.6.6/terraform_1.6.6_linux_amd64.zip -o terraform.zip && \
    unzip terraform.zip && \
    mv terraform /usr/local/bin/ && \
    rm terraform.zip

WORKDIR /app

# Copy the binary
COPY --from=builder /app/autostack .

# Copy migrations and templates
COPY pb_migrations ./pb_migrations
COPY templates ./templates

# Create directories for data and terraform workdir
RUN mkdir -p pb_data terraform-workdir pb_public

EXPOSE 8090

CMD ["./autostack", "serve", "--http=0.0.0.0:8090", "--publicDir=/app/pb_public"]