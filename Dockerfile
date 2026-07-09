FROM golang:1.26.5-alpine3.24 AS builder

WORKDIR /builder
RUN apk update && apk --no-cache add ca-certificates tzdata

COPY source/go.mod source/go.sum ./
RUN go mod download

COPY source/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o tommedag ./cmd/server

FROM ghcr.io/sollie/docker-upx:v5.2.0 AS upx
WORKDIR /upx
COPY --from=builder /builder/tommedag /upx/tommedag
RUN upx --best tommedag

FROM scratch AS final
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

WORKDIR /app
COPY --from=upx /upx/tommedag /app/

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/app/tommedag"]
