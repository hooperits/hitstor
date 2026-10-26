# Stage 1: Build
FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags="-X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${DATE}" \
    -o /out/ ./cmd/...

# Stage 2: Runtime
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S hitstor && adduser -S -G hitstor hitstor && \
    mkdir -p /var/lib/hitstor /etc/hitstor && \
    chown hitstor:hitstor /var/lib/hitstor
COPY --from=builder /out/* /usr/bin/
COPY packaging/docker/docker-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

USER hitstor
EXPOSE 8420 8421
VOLUME ["/var/lib/hitstor", "/etc/hitstor"]
ENTRYPOINT ["/entrypoint.sh"]
CMD ["serve"]
