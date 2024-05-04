FROM golang:1.22.2-bookworm as builder
  
COPY . /src
WORKDIR /src
RUN go build

FROM debian:bookworm-20240423

COPY --from=builder /src/generic-cdi-plugin /generic-cdi-plugin
  
ENTRYPOINT /generic-cdi-plugin
