FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY test/deploy/main.go ./main.go
RUN CGO_ENABLED=0 go build -o /fixture main.go
FROM alpine:3.23
COPY --from=build /fixture /fixture
HEALTHCHECK --interval=2s --timeout=2s --start-period=2s CMD wget -q -O /dev/null http://127.0.0.1:18081/api/nodes || exit 1
ENTRYPOINT ["/fixture"]
