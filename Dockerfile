####
#  Builder stage: Install dev dependencies → Generate deepcopy code/CRDs → Build binary
####
FROM golang:1.24-bookworm AS builder

# Install dependencies for libvirt-go (libvirt-dev, pkg-config)
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        git pkg-config libvirt-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# Download Go modules first (leverages Docker layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Install controller-gen (required by Kubebuilder for codegen)
RUN go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.0

# Copy project files
COPY . .

# Generate deepcopy methods and CRDs
RUN make generate

# Build the manager binary; CGO is required for libvirt-go
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -o manager cmd/main.go

####
# 2️⃣  Runtime stage: Slim execution image with only required libs
####
FROM ubuntu:22.04 AS runtime

ENV LANG=C.UTF-8 LC_ALL=C.UTF-8

# Only install client libraries required at runtime
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libvirt-clients libvirt0 && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /root/
COPY --from=builder /workspace/manager /manager

# Create non-root user and writable /config directory
RUN useradd -u 1000 capi && mkdir /config && chown capi /config
USER 1000:1000

# Optionally mark /config as a volume
VOLUME ["/config"]

# Entrypoint for the controller manager
ENTRYPOINT ["/manager"]
