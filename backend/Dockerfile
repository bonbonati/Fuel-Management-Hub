# syntax=docker/dockerfile:1
# One Dockerfile for every Go service; pick the service with --build-arg SERVICE=<name>
# (auth | fleet | station | payment | fueling). The result is a static binary on a
# distroless, non-root base image (no shell, no package manager).

FROM golang:1.27.3-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG SERVICE
RUN test -n "$SERVICE" || (echo "build-arg SERVICE is required" && exit 1)
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/${SERVICE}

FROM gcr.io/distroless/static-debian13:nonroot
ARG SERVICE
LABEL org.opencontainers.image.title="petro-${SERVICE}" \
      org.opencontainers.image.source="https://github.com/bonbonati/Fuel-Management-Hub"
COPY --from=build /out/service /service
USER nonroot:nonroot
EXPOSE 8080
# The binary probes its own /healthz (distroless has no curl/wget).
HEALTHCHECK --interval=10s --timeout=3s --start-period=30s --retries=3 CMD ["/service", "healthcheck"]
ENTRYPOINT ["/service"]
