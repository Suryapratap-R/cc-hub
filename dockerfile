# --- Build Stage ---
FROM golang:1.24.5-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go app, creating a static binary.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/pocketbase_app .



# --- Final Stage ---

FROM alpine:latest

# Add certificates for making outgoing TLS connections
RUN apk add --no-cache ca-certificates

# Set the working directory
WORKDIR /pb

# Copy the compiled binary from the builder stage
COPY --from=builder /app/pocketbase_app .

# Expose the port PocketBase will listen on
EXPOSE 8080

ENTRYPOINT [ "./pocketbase_app", "serve", "--http=0.0.0.0:8080" ]