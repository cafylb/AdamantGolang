ARG GO_VERSION=1.25.7
FROM golang:${GO_VERSION}-bookworm as builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go build -tags=webhook -ldflags="-s -w" -o /run-app ./wmain.go

FROM debian:bookworm

COPY --from=builder /run-app /usr/local/bin/
CMD ["run-app"]