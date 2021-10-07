FROM golang:1.17 AS builder

RUN apt-get -qq update && apt-get -yqq install upx

ENV GO111MODULE=on \
  CGO_ENABLED=0 \
  GOOS=linux \
  GOARCH=amd64

WORKDIR /src
COPY . .

RUN go build \
  -a \
  -trimpath \
  -ldflags "-s -w -extldflags '-static'" \
  -tags "osusergo,netgo,static,static_build" \
  -o /bin/app \
  .

RUN strip -s /bin/app
RUN upx -q -9 /bin/app

RUN echo "nobody:*:65534:65534:nobody:/:/bin/false" > /tmp/etc-passwd


FROM scratch

COPY --from=builder /tmp/etc-passwd /etc/passwd
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/app /bin/app
USER nobody

ENTRYPOINT ["/bin/app"]
