FROM golang:1.24 AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod ./
# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Use a minimal image for the runtime
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/server .
# Copy .env file if it exists (handled as part of COPY . above)

# Expose the application port
EXPOSE 8080

# Set environment variables (can be overridden at runtime)
ENV SERVER_HOST=0.0.0.0
ENV SERVER_PORT=8080
ENV OPENAI_MODEL=gpt-3.5-turbo
ENV OPENAI_BASE_URL=https://api.openai.com/v1

# Run the application
CMD ["./server"]
