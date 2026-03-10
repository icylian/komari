# Stage 1: Build frontend
FROM node:23-alpine AS frontend
WORKDIR /web
RUN apk add --no-cache git && \
    git clone https://github.com/komari-monitor/komari-web .
RUN npm install && npm run build

# Stage 2: Build backend
FROM golang:1.24-alpine AS builder
WORKDIR /src
RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Copy frontend build output
RUN mkdir -p public/defaultTheme/dist && \
    rm -rf public/defaultTheme/dist/*
COPY --from=frontend /web/dist/ public/defaultTheme/dist/
COPY --from=frontend /web/komari-theme.json public/defaultTheme/
RUN if [ -f /web/preview.png ]; then cp /web/preview.png public/defaultTheme/; fi

RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o komari

# Stage 3: Runtime
FROM alpine:3.21
WORKDIR /app

RUN apk add --no-cache tzdata

COPY --from=builder /src/komari /app/komari
RUN chmod +x /app/komari

ENV GIN_MODE=release
ENV KOMARI_DB_TYPE=sqlite
ENV KOMARI_DB_FILE=/app/data/komari.db
ENV KOMARI_DB_HOST=localhost
ENV KOMARI_DB_PORT=3306
ENV KOMARI_DB_USER=komari
ENV KOMARI_DB_PASS=
ENV KOMARI_DB_NAME=komari
ENV KOMARI_LISTEN=0.0.0.0:25774

EXPOSE 25774

CMD ["/app/komari", "server"]
