FROM golang:1.21 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o tx-mempool-simulator .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /app/tx-mempool-simulator /usr/local/bin/tx-mempool-simulator
EXPOSE 8545
ENTRYPOINT ["/usr/local/bin/tx-mempool-simulator"]
