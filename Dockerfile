FROM golang:1.12 AS builder

COPY . /barrelman
WORKDIR /barrelman
# Upgrade ca-certificates
RUN apt-get update && apt-get install ca-certificates && \
    make build

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /barrelman/barrelman .
ENTRYPOINT ["./barrelman"]