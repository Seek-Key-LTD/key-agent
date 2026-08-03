# adk-go / PicoOracle build
# Usage: make all        # compile for amd64
#         make oracle-agent  # just the oracle example
#         make clean
SHELL := /bin/bash
GO := go
REPO := github.com/Seek-Key-LTD/adk-go
OUTDIR := ./dist
ARCH ?= amd64
OS ?= linux

.PHONY: all clean oracle-agent test

all: $(OUTDIR)/oracle-agent-$(OS)-$(ARCH)

$(OUTDIR)/oracle-agent-$(OS)-$(ARCH): examples/oracle-agent/main.go
	@mkdir -p $(OUTDIR)
	GOOS=$(OS) GOARCH=$(ARCH) $(GO) build -trimpath -o $(OUTDIR)/oracle-agent-$(OS)-$(ARCH) examples/oracle-agent/

oracle-agent: all

test:
	$(GO) test -v -count=1 ./memory/... ./session/...

clean:
	rm -rf $(OUTDIR)
