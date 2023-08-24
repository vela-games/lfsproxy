FROM golang:1.20 AS build

WORKDIR /app

COPY go.* ./
RUN go mod download

COPY . ./

ENV CGO_ENABLED=0

RUN go build -v -o lfsproxy ./cmd/server.go

FROM alpine:3.17

WORKDIR /app

RUN apk add --no-cache libc6-compat gcompat

COPY --from=build /app/lfsproxy /app/lfsproxy
CMD ["/app/lfsproxy"]
