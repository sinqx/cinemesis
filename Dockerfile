# Dockerfile for the Go API with Air for development
FROM golang:1.24-alpine AS builder

# Set the working directory
WORKDIR /app

# Install necessary tools
RUN apk add --no-cache git
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
RUN go install github.com/air-verse/air@latest

# Copy go.mod and go.sum to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code (for development environment)
COPY . .

# Build the API binary (optional, for fallback or production)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/bin/api ./cmd/api

# Final stage: Use a lightweight Alpine image with development tools
FROM golang:1.24-alpine

# Install necessary packages
RUN apk --no-cache add ca-certificates tzdata

# Create a non-root user
RUN adduser -D -s /bin/sh appuser

# Set the working directory
WORKDIR /app

# Copy the migrate tool and air from the builder stage
COPY --from=builder /go/bin/migrate /usr/local/bin/migrate
COPY --from=builder /go/bin/air /usr/local/bin/air

# Copy migration files
COPY migrations /app/migrations

# Change ownership to the non-root user
RUN chown -R appuser:appuser /app
USER appuser

# Expose the port
EXPOSE 4000

# Command to run air (overridden by docker-compose for development)
CMD ["air"]

