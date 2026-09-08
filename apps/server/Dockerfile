# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS build
WORKDIR /src

COPY apps/server/go.mod ./
RUN go mod download

COPY apps/server/ ./
RUN mkdir -p /out/data && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bookharbor ./cmd/bookharbor

FROM scratch
COPY --from=build --chown=65532:65532 /out/bookharbor /bookharbor
COPY --from=build --chown=65532:65532 /out/data /data

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/bookharbor"]
