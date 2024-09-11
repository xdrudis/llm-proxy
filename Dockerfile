# docker build -t llm-proxy .
# docker run -d --restart unless-stopped -p 3030:3030 llm-proxy

#
# Builder stage
#
FROM golang:1.23.1-alpine3.20 AS builder

WORKDIR /app
COPY *.go go.mod go.sum ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o proxy .

#
# Runtime stage
#
FROM alpine:3.20

RUN adduser -D -g '' appuser

WORKDIR /app
RUN chown -R appuser:appuser /app
USER appuser
COPY --from=builder /app/proxy .

EXPOSE 3030
CMD ["./proxy"]
