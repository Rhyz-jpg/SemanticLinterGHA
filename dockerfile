# Stage 1: Build the Go application
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install git
RUN apk add --no-cache git

# Copy the Go module files and download dependencies
COPY go.mod go.sum* ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o semantic-linter .

# Stage 2: Create the final image
FROM alpine:latest

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/semantic-linter .

# Set the entrypoint for the container
ENTRYPOINT ["/app/semantic-linter"]