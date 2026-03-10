FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o mailfeed ./cmd/mailfeed

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /src/mailfeed /usr/local/bin/mailfeed

ENTRYPOINT ["mailfeed"]
