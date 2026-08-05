# Key Agent build
# Usage: make all         # compile for amd64
#         make k-agent    # just the k-agent binary
#         make clean
SHELL := /bin/bash
GO := go
REPO := github.com/Seek-Key-LTD/adk-go
OUTDIR := ./dist
ARCH ?= amd64
OS ?= linux

.PHONY: all clean k-agent test

all: $(OUTDIR)/k-agent-$(OS)-$(ARCH)

$(OUTDIR)/k-agent-$(OS)-$(ARCH): examples/oracle-agent/main.go
	@mkdir -p $(OUTDIR)
	GOOS=$(OS) GOARCH=$(ARCH) $(GO) build -trimpath -o $(OUTDIR)/k-agent-$(OS)-$(ARCH) examples/oracle-agent/

k-agent: all

test:
	$(GO) test -v -count=1 ./memory/... ./session/...

clean:
	rm -rf $(OUTDIR)