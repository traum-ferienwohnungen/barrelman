FROM golang:1.12 AS builder

COPY . /barrelman
WORKDIR /barrelman
RUN make build

FROM scratch
COPY --from=builder /barrelman/barrelman .
CMD ["./barrelman"]