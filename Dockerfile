FROM golang:alpine as builder

WORKDIR /go/src/app
COPY go.mod go.mod
COPY go.sum go.sum
ENV GO111MODULE=on
RUN apk add git
RUN go mod tidy
COPY . .
RUN go build -o /bin/etcd-admission-control main.go

FROM alpine:latest

WORKDIR /bin/
COPY --from=builder /bin/etcd-admission-control .

ENTRYPOINT ["/bin/etcd-admission-control"]
