# Build the Go binary, then ship it on a tiny base image.
# Build context is the backend/ directory (see render.yaml / docker build).
FROM golang:1.26-alpine AS build
WORKDIR /src

# Cache deps first so code changes don't re-download modules every build.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Static binary so it runs on a minimal image with no libc surprises.
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server

FROM alpine:3.20
# ca-certificates so outbound HTTPS (Finnhub, Yahoo) works.
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/server /app/server

ENV GIN_MODE=release
# Render injects PORT; this is just the local default.
EXPOSE 8080
CMD ["/app/server"]
