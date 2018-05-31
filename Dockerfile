FROM rust:1.26-slim AS rust

WORKDIR /build
COPY Cargo.lock Cargo.toml ./
COPY src ./src

RUN rustup target add x86_64-unknown-linux-musl
RUN cargo build --release --target x86_64-unknown-linux-musl

FROM golang:1.10-alpine AS go

RUN apk --update add \
        git \
        gcc \
        musl-dev \
        linux-headers
RUN go get -v github.com/genuinetools/img

FROM busybox
COPY --from=rust /build/target/x86_64-unknown-linux-musl/release/deployc-builder /usr/local/bin/deployc-builder
COPY --from=go /go/bin/img /usr/local/bin/img

CMD [ "deployc-builder" ]
