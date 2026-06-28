FROM golang:1.26-bookworm AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build \
      -ldflags "-s -w -extldflags '-static'" \
      -o /bin/app \
      .


FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/app /app

ENTRYPOINT ["/app"]
