# Build the SPA, then serve the static bundle with nginx.
FROM node:20-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm ci --no-audit --no-fund

COPY . .

# The API base URL is baked in at build time (Vite inlines import.meta.env).
ARG VITE_API_BASE_URL=http://localhost:8090
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL

RUN npm run build

FROM nginx:1.27-alpine AS runtime

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
# 127.0.0.1, not localhost: inside this image localhost resolves to ::1 only,
# while nginx listens on IPv4 — the check would fail against a healthy server.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
  CMD wget -qO- http://127.0.0.1/ >/dev/null 2>&1 || exit 1

CMD ["nginx", "-g", "daemon off;"]
