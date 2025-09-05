FROM golang:1.23-alpine as build
WORKDIR /src
COPY go.mod .
RUN go env -w GOPROXY=https://proxy.golang.org
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/projection-cache ./cmd/projection-cache

FROM alpine:3.18
# This is the crucial line: install curl and ca-certificates for health checks
RUN apk add --no-cache curl ca-certificates
COPY --from=build /app/projection-cache /usr/local/bin/projection-cache
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/projection-cache"]