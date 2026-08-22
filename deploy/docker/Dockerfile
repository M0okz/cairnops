# syntax=docker/dockerfile:1.9

FROM node:26-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS go-base
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

FROM go-base AS server-build
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X github.com/M0okz/cairnops/internal/version.Version=${VERSION} -X github.com/M0okz/cairnops/internal/version.Commit=${COMMIT} -X github.com/M0okz/cairnops/internal/version.BuildDate=${BUILD_DATE}" \
    -o /out/cairnops ./cmd/cairnops-server

FROM go-base AS worker-build
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X github.com/M0okz/cairnops/internal/version.Version=${VERSION} -X github.com/M0okz/cairnops/internal/version.Commit=${COMMIT} -X github.com/M0okz/cairnops/internal/version.BuildDate=${BUILD_DATE}" \
    -o /out/cairnops ./cmd/cairnops-worker

FROM go-base AS relay-build
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X github.com/M0okz/cairnops/internal/version.Version=${VERSION} -X github.com/M0okz/cairnops/internal/version.Commit=${COMMIT} -X github.com/M0okz/cairnops/internal/version.BuildDate=${BUILD_DATE}" \
    -o /out/cairnops ./cmd/cairnops-push-relay

FROM alpine:3.23 AS runtime
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S cairnops \
    && adduser -S -G cairnops -h /app cairnops \
    && mkdir -p /var/lib/cairnops \
    && mkdir -p /var/lib/cairnops-relay/registrations \
    && chown -R cairnops:cairnops /var/lib/cairnops /var/lib/cairnops-relay
WORKDIR /app

FROM runtime AS server
COPY --from=server-build --chown=cairnops:cairnops /out/cairnops /app/cairnops
COPY --from=web-build --chown=cairnops:cairnops /src/web/build /app/web
USER cairnops
ENTRYPOINT ["/app/cairnops"]

FROM runtime AS worker
COPY --from=worker-build --chown=cairnops:cairnops /out/cairnops /app/cairnops
USER cairnops
ENTRYPOINT ["/app/cairnops"]

FROM runtime AS relay
COPY --from=relay-build --chown=cairnops:cairnops /out/cairnops /app/cairnops
USER cairnops
ENTRYPOINT ["/app/cairnops"]
