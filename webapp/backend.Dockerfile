FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev pandoc
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /bin/server ./cmd/server
RUN go build -o /bin/worker ./cmd/worker

FROM alpine:3.20
RUN apk add --no-cache ca-certificates pandoc nodejs npm
COPY --from=builder /bin/server /usr/local/bin/server
COPY --from=builder /bin/worker /usr/local/bin/worker
EXPOSE 8080
CMD ["/usr/local/bin/server"]
