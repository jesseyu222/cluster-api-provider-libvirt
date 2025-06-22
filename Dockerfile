####
# 1️⃣  Builder stage：安裝 dev 套件 → 生成 deepcopy → 編譯二進位
####
FROM golang:1.24-bookworm AS builder

# libvirt-go 需要：libvirt-dev、pkg-config
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        git pkg-config libvirt-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

# 先下載 go modules
COPY go.mod go.sum ./
RUN go mod download

# 安裝 controller-gen（Kubebuilder 依賴）
RUN go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.0

# 複製所有原始碼
COPY . .

# 生成 deepcopy / CRD
RUN make generate

# 編譯；CGO_ENABLED=1 因 libvirt
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -o manager cmd/main.go

####
# 2️⃣  Runtime stage：只放執行期依賴，映像最精簡
####
FROM ubuntu:22.04 AS runtime

# 執行期只需 libvirt 動態庫（或僅 libvirt-clients 視需求）
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libvirt-clients libvirt0 && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /root/
COPY --from=builder /workspace/manager /manager

# 建立非 root 使用者
RUN useradd -u 1000 capi && mkdir /config && chown capi /config
USER 1000:1000

ENTRYPOINT ["/manager"]
