# Multi-stage build shared by the API, worker and migrate binaries.
# Build with:  docker build -f deployments/docker/go.Dockerfile --build-arg CMD=api .
FROM golang:1.25-alpine AS builder

ARG CMD=api

WORKDIR /src

# Cache dependencies separately from source.
COPY go.mod go.sum ./
RUN go mod download

COPY internal ./internal
COPY cmd ./cmd

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/app ./cmd/${CMD}

FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates tzdata curl && \
    adduser -D -u 10001 app

WORKDIR /app
COPY --from=builder /out/app /app/app
# Migrations are read at runtime by the migrate command and are small.
COPY migrations /app/migrations

USER app
ENTRYPOINT ["/app/app"]
