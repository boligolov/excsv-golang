# excsv-cli cross-platform build
#
# Usage (Git Bash, WSL, macOS, Linux):
#   make              # build for current OS/arch -> bin/excsv[.exe]
#   make rebuild      # flush Go cache + force full rebuild
#   make build-all    # cross-compile all targets -> bin/
#   make test
#   make clean

BINARY     := excsv
CMD        := ./cmd/excsv
BIN_DIR    := bin
MODULE     := github.com/boligolov/excsv-golang/internal/cli
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')")
LDFLAGS    := -s -w -X $(MODULE).Version=0.2.0 -X $(MODULE).BuildTime=$(BUILD_TIME)
GO_BUILD   := go build -trimpath -ldflags "$(LDFLAGS)"

ifeq ($(OS),Windows_NT)
	EXE     := .exe
	LOCAL   := $(BIN_DIR)/$(BINARY).exe
	RM      := powershell -NoProfile -Command "Remove-Item -Recurse -Force -ErrorAction SilentlyContinue"
	RMFILE  := powershell -NoProfile -Command "if (Test-Path '$(1)') { Remove-Item -Force '$(1)' -ErrorAction Stop }"
	MKDIR   := powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(BIN_DIR)' | Out-Null"
else
	EXE     :=
	LOCAL   := $(BIN_DIR)/$(BINARY)
	RM      := rm -rf
	RMFILE  := rm -f
	MKDIR   := mkdir -p $(BIN_DIR)
endif

.PHONY: all build rebuild build-all test clean list \
	build-windows-amd64 build-windows-arm64 \
	build-linux-amd64 build-linux-arm64 \
	build-darwin-amd64 build-darwin-arm64

all: build

build: $(BIN_DIR)
	$(call RMFILE,$(LOCAL))
	$(GO_BUILD) -o $(LOCAL) $(CMD)
	@echo -> $(LOCAL)

rebuild: $(BIN_DIR)
	@echo Flushing Go build cache...
	go clean -cache -testcache
	$(call RMFILE,$(LOCAL))
	$(GO_BUILD) -a -o $(LOCAL) $(CMD)
	@echo -> $(LOCAL)

$(BIN_DIR):
	$(MKDIR)

build-all: build-windows-amd64 build-windows-arm64 \
           build-linux-amd64 build-linux-arm64 \
           build-darwin-amd64 build-darwin-arm64
	@echo All binaries in $(BIN_DIR)/

build-windows-amd64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-windows-amd64.exe)
	GOOS=windows GOARCH=amd64 $(GO_BUILD) -o $(BIN_DIR)/$(BINARY)-windows-amd64.exe $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-windows-amd64.exe

build-windows-arm64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-windows-arm64.exe)
	GOOS=windows GOARCH=arm64 $(GO_BUILD) -o $(BIN_DIR)/$(BINARY)-windows-arm64.exe $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-windows-arm64.exe

build-linux-amd64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-linux-amd64)
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $(BIN_DIR)/$(BINARY)-linux-amd64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-linux-amd64

build-linux-arm64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-linux-arm64)
	GOOS=linux GOARCH=arm64 $(GO_BUILD) -o $(BIN_DIR)/$(BINARY)-linux-arm64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-linux-arm64

build-darwin-amd64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-darwin-amd64)
	GOOS=darwin GOARCH=amd64 $(GO_BUILD) -o $(BIN_DIR)/$(BINARY)-darwin-amd64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-darwin-amd64

build-darwin-arm64: $(BIN_DIR)
	$(call RMFILE,$(BIN_DIR)/$(BINARY)-darwin-arm64)
	GOOS=darwin GOARCH=arm64 $(GO_BUILD) -o $(BIN_DIR)/$(BINARY)-darwin-arm64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-darwin-arm64

test:
	go test ./...

clean:
	$(RM) $(BIN_DIR)

list:
	@echo Targets:
	@echo   make build       - local platform
	@echo   make rebuild     - flush Go cache + force full rebuild
	@echo   make build-all   - windows, linux, macOS \(amd64 + arm64\)
	@echo   make test
	@echo   make clean
