FROM golang:1.21-alpine as build
WORKDIR /app
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org && go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/consumer ./cmd/consumer

FROM alpine:3.18
COPY --from=build /app/consumer /usr/local/bin/consumer
ENTRYPOINT ["/usr/local/bin/consumer"]
