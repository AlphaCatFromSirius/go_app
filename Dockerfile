FROM golang:1.26 AS builder
WORKDIR /app
COPY go.mod go.sum /app
RUN go mod download
COPY ./main.go /app
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server /app/main.go 

FROM alpine:3.23
RUN apk --no-cache add curl
WORKDIR /app
COPY --from=builder /app/server /app/
EXPOSE 8888
ENTRYPOINT ["/app/server"]
