FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /restore-drill ./cmd/restore-drill

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /restore-drill /usr/local/bin/restore-drill
USER 65534:65534
ENTRYPOINT ["restore-drill"]
