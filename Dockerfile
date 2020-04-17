FROM golang:1.14.2 AS builder

ARG GO111MODULE=on
WORKDIR $GOPATH/src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go .
COPY cmd cmd
RUN CGO_ENABLED=0 go build -o /caddy -tags netgo,usergo -ldflags '-extldflags "-static"' ./cmd/caddy2

FROM alpine:3.11.5
COPY --from=builder /caddy /
RUN ["/caddy", "version"]
ENTRYPOINT ["/caddy"]
