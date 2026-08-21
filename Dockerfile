# syntax=docker/dockerfile:1
FROM golang:latest AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/acsd ./cmd/acsd
RUN CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=0 go build -o /out/seed-admin ./cmd/seed-admin

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/acsd /out/migrate /out/seed-admin ./
COPY migrations ./migrations
EXPOSE 8080 7547
ENTRYPOINT ["/app/acsd"]
