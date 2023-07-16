FROM golang:1.20 AS builder

RUN mkdir /src
WORKDIR /src

ADD . .
ARG VERSION
RUN CGO_ENABLED=0 go build -ldflags="-X main.Version=$VERSION" -v -o /server ./cmd/main/

FROM scratch

COPY --from=builder /server /server
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

CMD ["/server"]
