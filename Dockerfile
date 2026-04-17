FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev \
    mupdf-dev freetype-dev harfbuzz-dev jbig2dec-dev \
    libjpeg-turbo-dev openjpeg-dev gumbo-parser-dev zlib-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -tags "extlib static" -o /bin/pdf-translator ./cmd/translator

FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
    && addgroup -S appgroup && adduser -S appuser -G appgroup
COPY --from=builder /bin/pdf-translator /usr/local/bin/pdf-translator
COPY fonts/ /app/fonts/
RUN chown -R appuser:appgroup /app
WORKDIR /app
USER appuser

ENTRYPOINT ["pdf-translator"]
