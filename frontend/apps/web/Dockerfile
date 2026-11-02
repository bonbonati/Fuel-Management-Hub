# syntax=docker/dockerfile:1
# Web admin image. Build context: ./frontend (the pnpm workspace root).
#   docker build -f apps/web/Dockerfile -t petro/web ./frontend

FROM node:26.13.0-alpine3.24 AS build
RUN npm install -g pnpm@12.16.1
WORKDIR /src
# Install dependencies first (cached unless manifests or the lockfile change).
COPY pnpm-workspace.yaml package.json pnpm-lock.yaml tsconfig.base.json ./
COPY packages/shared/package.json packages/shared/
COPY apps/web/package.json apps/web/
RUN --mount=type=cache,id=pnpm,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile --filter @petro/web...
COPY packages/shared packages/shared
COPY apps/web apps/web
RUN pnpm --filter @petro/web build

FROM nginxinc/nginx-unprivileged:1.31.7-alpine3.24
LABEL org.opencontainers.image.title="petro-web" \
      org.opencontainers.image.source="https://github.com/bonbonati/Fuel-Management-Hub"
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/web/dist /usr/share/nginx/html
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
