# Stage 1: Build React SPA (pnpm via npm global - same major as .tool-versions; no Corepack)
FROM node:24-alpine AS frontend
RUN npm install -g pnpm@11.1.2
WORKDIR /app/web
COPY web/package.json web/pnpm-lock.yaml* ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
ARG VITE_STRIPE_PUBLISHABLE_KEY
ENV VITE_STRIPE_PUBLISHABLE_KEY=${VITE_STRIPE_PUBLISHABLE_KEY}
RUN pnpm run build

# Stage 2: Build Go binary
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/../internal/webembed/dist ./internal/webembed/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o booklab ./cmd/server

# Stage 3: Minimal runtime image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/booklab .
EXPOSE 8080
ENTRYPOINT ["./booklab"]
