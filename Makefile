# excsv-cli cross-platform build
#
# Usage (Git Bash, WSL, macOS, Linux):
#   make              # build for current OS/arch -> bin/excsv[.exe]
#   make build-all    # cross-compile all targets -> bin/
#   make test
#   make clean

BINARY  := excsv
CMD     := ./cmd/excsv
BIN_DIR := bin
LDFLAGS := -s -w

ifeq ($(OS),Windows_NT)
	EXE     := .exe
	LOCAL   := $(BIN_DIR)/$(BINARY).exe
	RM      := powershell -NoProfile -Command "Remove-Item -Recurse -Force -ErrorAction SilentlyContinue"
	MKDIR   := powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(BIN_DIR)' | Out-Null"
else
	EXE     :=
	LOCAL   := $(BIN_DIR)/$(BINARY)
	RM      := rm -rf
	MKDIR   := mkdir -p $(BIN_DIR)
endif

.PHONY: all build build-all test clean list \
	build-windows-amd64 build-windows-arm64 \
	build-linux-amd64 build-linux-arm64 \
	build-darwin-amd64 build-darwin-arm64

all: build

build: $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(LOCAL) $(CMD)
	@echo -> $(LOCAL)

$(BIN_DIR):
	$(MKDIR)

build-all: build-windows-amd64 build-windows-arm64 \
           build-linux-amd64 build-linux-arm64 \
           build-darwin-amd64 build-darwin-arm64
	@echo All binaries in $(BIN_DIR)/

build-windows-amd64: $(BIN_DIR)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-windows-amd64.exe $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-windows-amd64.exe

build-windows-arm64: $(BIN_DIR)
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-windows-arm64.exe $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-windows-arm64.exe

build-linux-amd64: $(BIN_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-linux-amd64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-linux-amd64

build-linux-arm64: $(BIN_DIR)
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-linux-arm64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-linux-arm64

build-darwin-amd64: $(BIN_DIR)
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-darwin-amd64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-darwin-amd64

build-darwin-arm64: $(BIN_DIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-darwin-arm64 $(CMD)
	@echo -> $(BIN_DIR)/$(BINARY)-darwin-arm64

test:
	go test ./...

clean:
	$(RM) $(BIN_DIR)

list:
	@echo Targets:
	@echo   make build       - local platform
	@echo   make build-all   - windows, linux, macOS \(amd64 + arm64\)
	@echo   make test
	@echo   make clean
