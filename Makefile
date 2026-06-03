# excsv-cli cross-platform build
#
# Usage (Git Bash, WSL, macOS, Linux):
#   make              # build for current OS/arch -> bin/excsv[.exe]
#   make rebuild      # flush Go cache + force full rebuild
#   make build-all    # cross-compile all targets -> bin/
#   make test
#   make clean
#
# ldflags must stay on one recipe line (single-quoted). Do not assign
# "go build -ldflags ..." to a Make variable — make re-splits on spaces.

BINARY   := excsv
GENCMD   := ./cmd/gencsv
CMD      := ./cmd/excsv
BIN_DIR  := bin

CLI_PKG     := $(shell go list -f '{{.ImportPath}}' ./internal/cli 2>/dev/null)
GENCLI_PKG  := $(shell go list -f '{{.ImportPath}}' ./internal/gencsv 2>/dev/null)
BUILD_TIME  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')")
LDFLAGS     := -s -w -X $(CLI_PKG).Version=0.2.0 -X $(CLI_PKG).BuildTime=$(BUILD_TIME)
GEN_LDFLAGS := -s -w -X $(GENCLI_PKG).Version=0.1.0 -X $(GENCLI_PKG).BuildTime=$(BUILD_TIME)

ifeq ($(CLI_PKG),)
$(error go list ./internal/cli failed — run make from the module root with Go installed)
endif
ifeq ($(GENCLI_PKG),)
$(error go list ./internal/gencsv failed — run make from the module root with Go installed)
endif

ifeq ($(OS),Windows_NT)
	EXE   := .exe
	LOCAL := $(BIN_DIR)/$(BINARY).exe
	GEN_LOCAL := $(BIN_DIR)/gencsv.exe
	RM    := powershell -NoProfile -Command "Remove-Item -Recurse -Force -ErrorAction SilentlyContinue"
define RMFILE
	powershell -NoProfile -Command "if (Test-Path '$(1)') { Remove-Item -Force '$(1)' -ErrorAction Stop }"
endef
	MKDIR := powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(BIN_DIR)' | Out-Null"
else
	EXE   :=
	LOCAL := $(BIN_DIR)/$(BINARY)
	GEN_LOCAL := $(BIN_DIR)/gencsv
	RM    := rm -rf
define RMFILE
	rm -f '$(1)'
endef
	MKDIR := mkdir -p $(BIN_DIR)
endif

.PHONY: all build rebuild build-all test clean list \
	sync-upstream sync-specs sync-fixtures \
	build-windows-amd64 build-windows-arm64 \
	build-linux-amd64 build-linux-arm64 \
	build-darwin-amd64 build-darwin-arm64

all: build

build: $(BIN_DIR)
	$(call RMFILE,$(LOCAL))
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(LOCAL) $(CMD)
	@echo -> $(LOCAL)
	$(call RMFILE,$(GEN_LOCAL))
	go build -trimpath -ldflags '$(GEN_LDFLAGS)' -o $(GEN_LOCAL) $(GENCMD)
	@echo -> $(GEN_LOCAL)

rebuild: $(BIN_DIR)
	@echo Flushing Go build cache...
	go clean -cache -testcache
	$(call RMFILE,$(LOCAL))
	go build -trimpath -a -ldflags '$(LDFLAGS)' -o $(LOCAL) $(CMD)
	@echo -> $(LOCAL)
	$(call RMFILE,$(GEN_LOCAL))
	go build -trimpath -a -ldflags '$(GEN_LDFLAGS)' -o $(GEN_LOCAL) $(GENCMD)
	@echo -> $(GEN_LOCAL)

$(BIN_DIR):
	$(MKDIR)

build-all: build-windows-amd64 build-windows-arm64 \
           build-linux-amd64 build-linux-arm64 \
           build-darwin-amd64 build-darwin-arm64
	@echo All binaries in $(BIN_DIR)/

build-windows-amd64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-windows-amd64.exe)
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY)-windows-amd64.exe $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-windows-amd64.exe

build-windows-arm64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-windows-arm64.exe)
	GOOS=windows GOARCH=arm64 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY)-windows-arm64.exe $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-windows-arm64.exe

build-linux-amd64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-linux-amd64)
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY)-linux-amd64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-linux-amd64

build-linux-arm64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-linux-arm64)
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY)-linux-arm64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-linux-arm64

build-darwin-amd64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-darwin-amd64)
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY)-darwin-amd64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-darwin-amd64

build-darwin-arm64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-darwin-arm64)
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY)-darwin-arm64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-darwin-arm64

test:
	go test ./...

sync-upstream:
	bash scripts/sync-upstream.sh

sync-specs:
	bash scripts/sync-upstream.sh --specs-only

sync-fixtures:
	bash scripts/sync-upstream.sh --fixtures-only

clean:
	$(RM) $(BIN_DIR)

list:
	@echo Targets:
	@echo   make build       - excsv + gencsv for local platform
	@echo   make rebuild     - flush Go cache + force full rebuild
	@echo   make build-all   - windows, linux, macOS \(amd64 + arm64\)
	@echo   make test
	@echo   make sync-upstream   - download spec snapshots + fixtures from upstream
	@echo   make sync-specs      - spec snapshots + fixtures.yaml only
	@echo   make sync-fixtures   - fixture files from local fixtures.yaml
	@echo   make clean
