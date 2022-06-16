FROM golang:1.18 as builder

WORKDIR /opt
WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 go build -v -o /usr/local/bin/archaware-controller ./...

FROM alpine:3.16
COPY --from=builder /usr/local/bin/archaware-controller /usr/local/bin/archaware-controller

CMD ["/usr/local/bin/archaware-controller"]
