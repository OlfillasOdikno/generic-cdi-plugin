FROM golang:1.22.2-bookworm AS builder

COPY go.mod go.sum ./
RUN go mod download

RUN mkdir /out
RUN mkdir -m 1755 /out/tmp

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/generic-cdi-plugin

FROM scratch

COPY --from=builder /out/. /

ENTRYPOINT ["/generic-cdi-plugin"]
