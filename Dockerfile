# syntax=docker/dockerfile:1
# Versi Go DI-PIN (bukan :latest) supaya build reprodusibel dan hasil
# govulncheck bisa dipertanggungjawabkan. 1.26.9 = patch pertama yang menutup 12
# CVE stdlib (net/http, crypto/tls, html/template, net/textproto) yang ada di 1.26.8
# (govulncheck 2026-10-15). Naikkan patch ini tiap govulncheck melaporkan temuan stdlib.
FROM golang:1.26.9 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/acsd ./cmd/acsd
RUN CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=0 go build -o /out/seed-admin ./cmd/seed-admin

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
# Jalan sebagai user non-root: proses ini menghadap jaringan CPE/BSS dan tidak
# butuh hak apa pun (port 8080/7547 > 1024, tidak menulis ke filesystem).
RUN useradd --system --uid 10001 --no-create-home --shell /usr/sbin/nologin acs
WORKDIR /app
COPY --from=build /out/acsd /out/migrate /out/seed-admin ./
COPY migrations ./migrations
USER acs
EXPOSE 8080 7547
ENTRYPOINT ["/app/acsd"]
