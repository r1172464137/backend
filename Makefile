NAME="ppanel-server"
BINDIR=bin
# Keep the injected version free of the tag's leading "v"; the release pipelines
# strip it too, and cmd/run.go prints its own "v" prefix.
VERSION=$(shell git describe --tags 2>/dev/null | sed 's/^v//')
ifeq ($(strip $(VERSION)),)
VERSION=unknown version
endif
CHANNEL?=dev
# Empty means now. CI passes one value so every artifact of a run agrees.
BUILD_TIME?=
# script/ldflags.sh is the only definition of the injected build metadata; the
# Dockerfile and the release workflow call it too. Expanded once, so every
# binary of one make run carries the same build time.
LDFLAGS:=$(shell VERSION='$(VERSION)' CHANNEL='$(CHANNEL)' BUILD_TIME='$(BUILD_TIME)' sh script/ldflags.sh)
ifeq ($(strip $(LDFLAGS)),)
$(error script/ldflags.sh failed; check VERSION, CHANNEL and BUILD_TIME)
endif
GOBUILD=CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)"

# Pinned developer tools. CI installs the same versions through these targets.
TOOLS_BIN := $(CURDIR)/bin/tools
GOLANGCI_LINT_VERSION ?= v2.15.0
GOIMPORTS_VERSION ?= v0.44.0
GOVULNCHECK_VERSION ?= v1.8.0
GORELEASER_VERSION ?= v2.18.2
# protoc reports this release as "libprotoc 3.21.12", which is the version
# recorded in the generated files' headers.
PROTOC_VERSION ?= 21.12
GOLANGCI_LINT := $(TOOLS_BIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
GOIMPORTS := $(TOOLS_BIN)/goimports-$(GOIMPORTS_VERSION)
PROTOC_GEN_GO := $(TOOLS_BIN)/protoc-gen-go
PROTOC ?= protoc
PROTO_FILES := api/server/v1/server.proto
# lint-new reports issues on lines changed since this revision; HEAD covers
# uncommitted work, a branch's merge base (LINT_BASE=origin/dev) covers a PR.
LINT_BASE ?= HEAD

PLATFORM_LIST = \
	darwin-amd64 \
	darwin-amd64-v3 \
	darwin-arm64 \
	linux-386 \
	linux-amd64 \
	linux-amd64-v3 \
	linux-armv5 \
	linux-armv6 \
	linux-armv7 \
	linux-arm64 \

WINDOWS_ARCH_LIST = \
	windows-386 \
	windows-amd64 \
	windows-amd64-v3 \
	windows-arm64 \
	windows-armv7

all: linux-amd64 darwin-amd64 windows-amd64 # Most used

perf:
	bash scripts/perf/bench.sh

.PHONY: check fmt fmt-files fmt-check vet lint lint-new test test-race vulncheck proto proto-check ldflags

# check is the fast local subset of CI: formatting, vet, lint and the unit
# tests (CI adds the database-backed and -race runs, govulncheck and the
# generated-code check).
check: fmt-check vet lint test

fmt: $(GOIMPORTS)
	$(GOIMPORTS) -w .

# Formats only FILES; the pre-commit hook passes the staged Go files.
fmt-files: $(GOIMPORTS)
	$(GOIMPORTS) -w $(FILES)

fmt-check: $(GOIMPORTS)
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then echo "gofmt would reformat:"; echo "$$unformatted"; exit 1; fi
	@unformatted="$$($(GOIMPORTS) -l .)"; \
	if [ -n "$$unformatted" ]; then echo "goimports would rewrite (run make fmt):"; echo "$$unformatted"; exit 1; fi

vet:
	go vet ./...

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

# Reports only issues on lines changed since LINT_BASE: the pre-commit hook's
# quick pass. CI lints the whole tree.
lint-new: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --new-from-rev=$(LINT_BASE) ./...

test:
	go test ./...

test-race:
	go test -race ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

# Regenerates the protobuf bindings with the pinned protoc and the
# protoc-gen-go version go.mod requires, which the generated headers record.
proto: $(PROTOC_GEN_GO) $(GOIMPORTS)
	@found="$$($(PROTOC) --version 2>/dev/null)"; \
	if [ "$$found" != "libprotoc 3.$(PROTOC_VERSION)" ]; then \
		echo "protoc $(PROTOC_VERSION) (libprotoc 3.$(PROTOC_VERSION)) required, found: $${found:-none}"; exit 1; fi
	$(PROTOC) --plugin=protoc-gen-go=$(PROTOC_GEN_GO) --go_out=paths=source_relative:. $(PROTO_FILES)
	# Keep generator output in the same import format as fmt and the commit hook.
	$(GOIMPORTS) -w $(PROTO_FILES:.proto=.pb.go)

# Fails when the committed bindings differ from what the .proto files generate.
proto-check: proto
	git diff --exit-code -- api/

ldflags:
	@echo "$(LDFLAGS)"

# print-VAR prints a variable, so workflows read the pinned versions from here:
#   make -s print-PROTOC_VERSION
print-%:
	@echo '$($*)'

$(GOLANGCI_LINT):
	GOBIN=$(TOOLS_BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	mv $(TOOLS_BIN)/golangci-lint $@

$(GOIMPORTS):
	GOBIN=$(TOOLS_BIN) go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
	mv $(TOOLS_BIN)/goimports $@

$(PROTOC_GEN_GO): go.mod go.sum
	go build -o $@ google.golang.org/protobuf/cmd/protoc-gen-go

darwin-amd64:
	GOARCH=amd64 GOOS=darwin $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

darwin-amd64-v3:
	GOARCH=amd64 GOOS=darwin GOAMD64=v3 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

darwin-arm64:
	GOARCH=arm64 GOOS=darwin $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-386:
	GOARCH=386 GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-amd64:
	GOARCH=amd64 GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-amd64-v3:
	GOARCH=amd64 GOOS=linux GOAMD64=v3 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-armv5:
	GOARCH=arm GOOS=linux GOARM=5 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-armv6:
	GOARCH=arm GOOS=linux GOARM=6 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-armv7:
	GOARCH=arm GOOS=linux GOARM=7 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

linux-arm64:
	GOARCH=arm64 GOOS=linux $(GOBUILD) -o $(BINDIR)/$(NAME)-$@

windows-386:
	GOARCH=386 GOOS=windows $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

windows-amd64:
	GOARCH=amd64 GOOS=windows $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

windows-amd64-v3:
	GOARCH=amd64 GOOS=windows GOAMD64=v3 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

windows-arm64:
	GOARCH=arm64 GOOS=windows $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe

windows-armv7:
	GOARCH=arm GOOS=windows GOARM=7 $(GOBUILD) -o $(BINDIR)/$(NAME)-$@.exe


gz_releases=$(addsuffix .gz, $(PLATFORM_LIST))
zip_releases=$(addsuffix .zip, $(WINDOWS_ARCH_LIST))

$(gz_releases): %.gz : %
	chmod +x $(BINDIR)/$(NAME)-$(basename $@)
	gzip -f -S -$(VERSION).gz $(BINDIR)/$(NAME)-$(basename $@)

$(zip_releases): %.zip : %
	zip -m -j $(BINDIR)/$(NAME)-$(basename $@)-$(VERSION).zip $(BINDIR)/$(NAME)-$(basename $@).exe
