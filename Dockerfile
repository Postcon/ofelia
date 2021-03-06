ARG ARCH=amd64

FROM golang:1.10.0 AS builder-amd64
RUN apt-get update && apt-get install -y ca-certificates

FROM arm32v6/golang:1.10.0-alpine AS builder-arm32v6
RUN apk add --no-cache tzdata ca-certificates

FROM builder-${ARCH} AS builder

WORKDIR ${GOPATH}/src/github.com/Postcon/ofelia
COPY . ${GOPATH}/src/github.com/Postcon/ofelia

ENV CGO_ENABLED 0
ENV GOOS linux

RUN go get -v ./...
RUN go build -a -installsuffix cgo -ldflags '-w  -extldflags "-static"' -o /go/bin/ofelia .

FROM scratch

COPY --from=builder /go/bin/ofelia /usr/bin/ofelia
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

VOLUME /etc/ofelia/
ENTRYPOINT ["/usr/bin/ofelia"]

CMD ["daemon", "--config", "/etc/ofelia/config.ini"]
