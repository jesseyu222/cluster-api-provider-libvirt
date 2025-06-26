FROM golang:1.24-bookworm AS builder

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        git pkg-config libvirt-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace


COPY go.mod go.sum ./
RUN go mod download

RUN go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.0


COPY . .


RUN make generate

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o manager cmd/main.go


FROM ubuntu:22.04 AS runtime

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libvirt-clients libvirt0 && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /root/
COPY --from=builder /workspace/manager /manager

RUN useradd -u 1000 capi && mkdir /config && chown capi /config
USER 1000:1000

ENTRYPOINT ["/manager"]
