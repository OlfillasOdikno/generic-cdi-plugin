FROM golang:1.22.2-bookworm as builder

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /generic-cdi-plugin

FROM scratch

COPY --from=builder /generic-cdi-plugin /

ENTRYPOINT /generic-cdi-plugin
