FROM golang:1.18 as builder

WORKDIR /opt
RUN mkdir -p /opt/containerd && \
    curl -LO \
    https://github.com/containerd/containerd/releases/download/v1.6.4/containerd-1.6.4-linux-amd64.tar.gz && \
  curl -LO \
    https://github.com/containerd/containerd/releases/download/v1.6.4/containerd-1.6.4-linux-amd64.tar.gz.sha256sum && \
  sha256sum \
    -c containerd-1.6.4-linux-amd64.tar.gz.sha256sum && \
  tar Cxzvf /opt/containerd containerd-1.6.4-linux-amd64.tar.gz
WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN go build -v -o /usr/local/bin/archaware-operator ./...

FROM golang:1.18
WORKDIR /go
COPY --from=builder /opt/containerd /opt/containerd
ENV PATH="/opt/containerd/bin:${PATH}"
COPY --from=builder /usr/local/bin/archaware-operator /usr/local/bin/archaware-operator

CMD ["archaware-operator"]
