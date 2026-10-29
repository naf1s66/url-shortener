FROM golang:1.25-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o url-shortener .

FROM alpine:3.22 AS runtime

WORKDIR /app

COPY --from=builder /app/url-shortener /app/url-shortener

COPY --from=builder /app/docs /app/docs

RUN apk add --no-cache ca-certificates

USER 10001:10001

EXPOSE 8080

CMD ["/app/url-shortener"]