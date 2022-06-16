FROM golang:1.18 as builder

WORKDIR /opt
WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go build -v -o /usr/local/bin/archaware-controller ./...

FROM alpine:3.16
COPY --from=builder /usr/local/bin/archaware-operator /usr/local/bin/archaware-controller

CMD ["archaware-controller"]
