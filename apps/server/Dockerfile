# syntax=docker/dockerfile:1

# The admin console is a React app whose build output the Go binary embeds.
# Vite writes it to ../server/internal/httpapi/adminui relative to /admin.
FROM node:22-alpine AS admin
WORKDIR /admin
COPY apps/admin/package.json apps/admin/package-lock.json ./
RUN npm ci
COPY apps/admin/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src

COPY apps/server/go.mod ./
RUN go mod download

COPY apps/server/ ./
COPY --from=admin /server/internal/httpapi/adminui ./internal/httpapi/adminui
RUN mkdir -p /out/data /out/tmp && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bookharbor ./cmd/bookharbor

# slim: just the binary. Everything works except importing MOBI/AZW3, which needs Calibre.
# Build it with `docker build --target slim`.
FROM scratch AS slim
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65532:65532 /out/bookharbor /bookharbor
COPY --from=build --chown=65532:65532 /out/data /data
COPY --from=build --chown=65532:65532 /out/tmp /tmp

USER 65532:65532
ENV TMPDIR=/tmp
EXPOSE 8080
ENTRYPOINT ["/bookharbor"]

# full (the default): adds Calibre's ebook-convert, so MOBI and AZW3 books (common from
# Shelfmark) are converted to EPUB on import. Several hundred MB larger than slim.
FROM debian:bookworm-slim AS full
RUN apt-get update && \
    apt-get install -y --no-install-recommends calibre ca-certificates && \
    rm -rf /var/lib/apt/lists/* && \
    install -d -o 65532 -g 65532 /home/bookharbor
COPY --from=build --chown=65532:65532 /out/bookharbor /bookharbor
COPY --from=build --chown=65532:65532 /out/data /data

USER 65532:65532
# Calibre keeps its settings under HOME and needs no display.
ENV HOME=/home/bookharbor QT_QPA_PLATFORM=offscreen
EXPOSE 8080
ENTRYPOINT ["/bookharbor"]
