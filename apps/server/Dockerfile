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

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65532:65532 /out/bookharbor /bookharbor
COPY --from=build --chown=65532:65532 /out/data /data
COPY --from=build --chown=65532:65532 /out/tmp /tmp

USER 65532:65532
ENV TMPDIR=/tmp
EXPOSE 8080
ENTRYPOINT ["/bookharbor"]
