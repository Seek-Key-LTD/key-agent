# Key Agent build — 双产物物理分包（ROADMAP-GUEST-HOST-SPLIT Phase 1）
#   make host    # host 产物：含平台凭据 Provider（Vault 等）
#   make guest   # guest 产物：go build -tags guest，二进制零 Vault 代码/字符串
#   make all     # 两者 + SHA256SUMS
#   make k-agent # 兼容旧入口，等价 host
#   make test / make clean
SHELL := /bin/bash
GO := go
REPO := github.com/Seek-Key-LTD/adk-go
OUTDIR := ./dist
ARCH ?= amd64
OS ?= linux
BLD := $(OS)-$(ARCH)

.PHONY: all host guest k-agent test clean sha256

all: host guest sha256

host: $(OUTDIR)/k-agent-host-$(BLD)

guest: $(OUTDIR)/k-agent-guest-$(BLD)

$(OUTDIR)/k-agent-host-$(BLD):
	@mkdir -p $(OUTDIR)
	GOOS=$(OS) GOARCH=$(ARCH) $(GO) build -trimpath -ldflags '-s -w' -o $@ ./cmd/agent_dispatcher

$(OUTDIR)/k-agent-guest-$(BLD):
	@mkdir -p $(OUTDIR)
	GOOS=$(OS) GOARCH=$(ARCH) $(GO) build -trimpath -ldflags '-s -w' -tags guest -o $@ ./cmd/agent_dispatcher

k-agent: host

test:
	$(GO) test -count=1 ./internal/secrets/... ./memory/... ./session/...
	$(GO) test -count=1 -tags guest ./internal/secrets/...

# guest 产物纯度门禁：编译后的二进制里不允许出现任何 vault 字样
verify-guest: guest
	@strings $(OUTDIR)/k-agent-guest-$(BLD) | grep -i vault && { echo "❌ guest 二进制含 vault 引用"; exit 1; } || echo "✓ guest 二进制零 vault 引用"

sha256:
	@mkdir -p $(OUTDIR)
	cd $(OUTDIR) && sha256sum k-agent-* > SHA256SUMS && cat SHA256SUMS

clean:
	rm -rf $(OUTDIR)
