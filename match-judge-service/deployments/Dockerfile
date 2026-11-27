FROM golang:1.27-alpine AS base
WORKDIR /app
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum* ./
RUN go mod download

FROM base AS development
RUN go install github.com/air-verse/air@latest
CMD ["air", "-c", ".air.toml"]

FROM base AS builder
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/worker ./cmd/worker

FROM alpine:3.20 AS production
WORKDIR /app
COPY --from=builder /app/worker /app/worker
CMD ["/app/worker"]
