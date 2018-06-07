FROM golang:1.10-alpine AS go

RUN apk --update add \
        curl \
        git \
        gcc \
        musl-dev \
        linux-headers

WORKDIR /artifacts
RUN set -ex && \
    curl -sSL https://github.com/genuinetools/img/releases/download/v0.3.9/img-linux-amd64 -o /artifacts/img && \
    export SHASUM=$(curl -sSL https://github.com/genuinetools/img/releases/download/v0.3.9/img-linux-amd64.sha256 | awk '{ print $1 }') && \
    if [ "$SHASUM" != "$(sha256sum /artifacts/img | awk '{ print $1 }')" ]; then echo "sha256sum mismatch!"; exit 1; fi && \
    chmod a+x /artifacts/img && \
    curl -sSL https://misc.j3ss.co/tmp/runc -o /artifacts/runc && \
    chmod a+x /artifacts/runc

WORKDIR /go/src/deployc-builder
COPY main.go dockerfiles.go ./
RUN go get -d ./...
RUN GOOS=linux GOARCH=amd64 go install --ldflags '-linkmode external -extldflags "-static"'
RUN mv /go/bin/deployc-builder /artifacts

COPY run.sh /artifacts/

FROM alpine:3.7
RUN apk --update --no-cache add git ca-certificates
COPY --from=go /artifacts/* /usr/local/bin/
CMD [ "run.sh" ]
