ARG ARCH=amd64

FROM golang:1.10.0 AS builder-amd64
RUN apt-get update && apt-get install -y ca-certificates

FROM arm32v6/golang:1.10.0-alpine AS builder-arm32v6
RUN apk add --no-cache ca-certificates

FROM builder-${ARCH} AS builder

WORKDIR ${GOPATH}/src/github.com/mcuadros/ofelia
COPY . ${GOPATH}/src/github.com/mcuadros/ofelia

ENV CGO_ENABLED 0
ENV GOOS linux

RUN go get -v ./...
RUN go build -a -installsuffix cgo -ldflags '-w  -extldflags "-static"' -o /go/bin/ofelia .

FROM scratch

COPY --from=builder /go/bin/ofelia /usr/bin/ofelia
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

VOLUME /etc/ofelia/
ENTRYPOINT ["/usr/bin/ofelia"]

CMD ["daemon", "--config", "/etc/ofelia/config.ini"]
