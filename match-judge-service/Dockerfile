FROM golang:1.27-alpine AS base
WORKDIR /app
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum* ./
RUN go mod download

FROM base AS development
RUN go install github.com/air-verse/air@latest
COPY . .
CMD ["air", "-c", ".air.toml"]

FROM base AS builder
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server .

FROM alpine:3.20 AS production
WORKDIR /app
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
COPY --from=builder /app/server /app/server
USER appuser
CMD ["/app/server"]
