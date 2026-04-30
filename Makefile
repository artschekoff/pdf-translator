BINARY       := pdf-translator
BUILD_DIR    := bin
MAIN         := ./cmd/translator
COMPOSE      := docker compose
COMPOSE_FILE := docker-compose.yml
# Windows uses system MuPDF from MSYS2 (extlib=external lib, static=link statically)
WIN_GO_TAGS  := -tags "extlib static"
WIN_CGO_CFLAGS  := -I/c/msys64/mingw64/include
WIN_CGO_LDFLAGS := -L/c/msys64/mingw64/lib -lmupdf -lmupdf-third -lfreetype -lharfbuzz -ljbig2dec -ljpeg -lopenjp2 -lgumbo -lz -lgdi32 -lcomdlg32 -lm

# macOS uses bundled MuPDF from go-fitz (no extlib tag needed, no brew dependency)
MAC_GO_TAGS  := -tags "static"

export CGO_ENABLED := 1

# ─── Local Development ──────────────────────────────────────

.PHONY: build-win build-mac build-windows build-linux build-darwin build-all run-win run-mac clean deps

build-win:
	@mkdir -p $(BUILD_DIR)
	CGO_CFLAGS="$(WIN_CGO_CFLAGS)" CGO_LDFLAGS="$(WIN_CGO_LDFLAGS)" \
		go build $(WIN_GO_TAGS) -o $(BUILD_DIR)/$(BINARY).exe $(MAIN)
	@echo "Built $(BUILD_DIR)/$(BINARY).exe"

build-mac:
	@mkdir -p $(BUILD_DIR)
	go build $(MAC_GO_TAGS) -o $(BUILD_DIR)/$(BINARY)-darwin $(MAIN)
	@echo "Built $(BUILD_DIR)/$(BINARY)-darwin"

build-windows:
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc \
		go build $(MAC_GO_TAGS) -o $(BUILD_DIR)/$(BINARY).exe $(MAIN)
	@echo "Built $(BUILD_DIR)/$(BINARY).exe"

build-linux:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 \
		go build $(MAC_GO_TAGS) -o $(BUILD_DIR)/$(BINARY)-linux $(MAIN)
	@echo "Built $(BUILD_DIR)/$(BINARY)-linux"

build-darwin:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 \
		go build $(MAC_GO_TAGS) -o $(BUILD_DIR)/$(BINARY)-darwin $(MAIN)
	@echo "Built $(BUILD_DIR)/$(BINARY)-darwin"

build-all: build-windows build-linux build-darwin
	@echo "All binaries in $(BUILD_DIR)/"

run-win: build-win
	./$(BUILD_DIR)/$(BINARY).exe $(ARGS)

run-mac: build-mac
	./$(BUILD_DIR)/$(BINARY)-darwin $(ARGS)

clean:
	rm -rf $(BUILD_DIR)
	rm -rf tmp/

deps:
	go mod download
	go mod tidy

# ─── Code Quality ───────────────────────────────────────────

.PHONY: test lint fmt vet validate

test:
	go test $(MAC_GO_TAGS) ./... -cover -count=1

lint:
	golangci-lint run

fmt:
	gofumpt -l -w .

vet:
	go vet ./...

validate: fmt vet lint test

# ─── Docker: OCR Services (local dev mode) ──────────────────

.PHONY: services-up services-down services-up-all services-logs services-status

services-up:
	$(COMPOSE) up -d paddleocr
	@echo "PaddleOCR running at http://localhost:8051"

services-up-all:
	$(COMPOSE) --profile tesseract up -d
	@echo "PaddleOCR: http://localhost:8051"
	@echo "Tesseract: http://localhost:8052"

services-down:
	$(COMPOSE) --profile tesseract down

services-logs:
	$(COMPOSE) logs -f paddleocr tesseract

services-status:
	$(COMPOSE) ps

# ─── Docker: Full Stack (everything in Docker) ──────────────

.PHONY: docker-build docker-up docker-up-all docker-down docker-run docker-logs

docker-build:
	$(COMPOSE) build app

docker-up: docker-build
	$(COMPOSE) --profile app up -d
	@echo "All services running in Docker"

docker-up-all: docker-build
	$(COMPOSE) --profile app --profile tesseract up -d
	@echo "All services (including Tesseract) running in Docker"

docker-down:
	$(COMPOSE) --profile app --profile tesseract down

docker-run: docker-build
	@mkdir -p output
	$(COMPOSE) --profile app run --rm app \
		translate /data/input/$(INPUT) \
		--output /data/output/$(or $(OUTPUT),$(basename $(INPUT))_translated.pdf) \
		--to $(TO) \
		$(if $(FROM),--from $(FROM),) \
		$(if $(PAGES),--pages $(PAGES),) \
		$(if $(WORKERS),--workers $(WORKERS),)

docker-logs:
	$(COMPOSE) --profile app --profile tesseract logs -f

# ─── Health Checks ──────────────────────────────────────────

.PHONY: health

health:
	@echo "Checking PaddleOCR..."
	@curl -sf http://localhost:8051/health > /dev/null && echo "  PaddleOCR: OK" || echo "  PaddleOCR: NOT RUNNING"
	@echo "Checking Tesseract..."
	@curl -sf http://localhost:8052/health > /dev/null && echo "  Tesseract: OK" || echo "  Tesseract: NOT RUNNING (optional)"
	@echo "Checking .env..."
	@test -f .env && echo "  .env: OK" || echo "  .env: MISSING (copy from .env.example)"

# ─── Setup & Cleanup ───────────────────────────────────────

.PHONY: setup setup-mac clean-jobs clean-jobs-mac download-fonts download-fonts-mac

setup:
	@echo "=== Windows Prerequisites ==="
	@echo "1. MSYS2 with MinGW-w64 gcc + MuPDF:"
	@echo "   winget install -e --id MSYS2.MSYS2"
	@echo "   pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-libmupdf mingw-w64-x86_64-mupdf-tools"
	@echo "2. Add C:\\msys64\\mingw64\\bin to your PATH"
	@echo ""
	@test -f .env || cp .env.example .env
	@echo "Edit .env and set your OPENAI_API_KEY"
	go mod download
	$(COMPOSE) up -d paddleocr
	@echo ""
	@echo "Setup complete. Run: make build-win && make run-win ARGS=\"translate input.pdf --to spanish\""

setup-mac:
	@echo "=== macOS Prerequisites ==="
	@echo "MuPDF is bundled — no extra system dependencies required."
	@echo ""
	@test -f .env || cp .env.example .env
	@echo "Edit .env and set your OPENAI_API_KEY"
	go mod download
	$(COMPOSE) up -d paddleocr
	@echo ""
	@echo "Setup complete. Run: make build-mac && make run-mac ARGS=\"translate input.pdf --to spanish\""

clean-jobs:
	./$(BUILD_DIR)/$(BINARY).exe cleanup

clean-jobs-mac:
	./$(BUILD_DIR)/$(BINARY)-darwin cleanup

download-fonts:
	./$(BUILD_DIR)/$(BINARY).exe download-fonts

download-fonts-mac:
	./$(BUILD_DIR)/$(BINARY)-darwin download-fonts

# ─── Help ───────────────────────────────────────────────────

.PHONY: help

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Local Development (Windows):"
	@echo "  build-win        Compile the CLI binary for Windows (.exe)"
	@echo "  run-win          Build and run on Windows (e.g. make run-win ARGS=\"translate input.pdf --to es\")"
	@echo "  setup            First-time setup for Windows"
	@echo "  clean-jobs       Remove orphaned temp files (Windows)"
	@echo "  download-fonts   Pre-download all font variants (Windows)"
	@echo ""
	@echo "Local Development (macOS):"
	@echo "  build-mac        Compile the CLI binary for macOS (MuPDF bundled, no extra deps)"
	@echo "  run-mac          Build and run on macOS (e.g. make run-mac ARGS=\"translate input.pdf --to es\")"
	@echo "  setup-mac        First-time setup for macOS"
	@echo "  clean-jobs-mac   Remove orphaned temp files (macOS)"
	@echo "  download-fonts-mac Pre-download all font variants (macOS)"
	@echo ""
	@echo "Cross-compilation:"
	@echo "  build-windows    Cross-compile .exe for Windows (amd64)"
	@echo "  build-linux      Cross-compile for Linux (amd64)"
	@echo "  build-darwin     Cross-compile for macOS (arm64)"
	@echo "  build-all        Build for all platforms"
	@echo "  clean            Remove build artifacts"
	@echo "  deps             Download Go dependencies"
	@echo ""
	@echo "Code Quality:"
	@echo "  test             Run tests with coverage"
	@echo "  lint             Run golangci-lint"
	@echo "  fmt              Format code with gofumpt"
	@echo "  vet              Run go vet"
	@echo "  validate         Run all checks (fmt -> vet -> lint -> test)"
	@echo ""
	@echo "Docker (OCR services for local dev):"
	@echo "  services-up      Start PaddleOCR"
	@echo "  services-up-all  Start PaddleOCR + Tesseract"
	@echo "  services-down    Stop all Docker services"
	@echo "  services-logs    Tail OCR service logs"
	@echo "  services-status  Show Docker service status"
	@echo ""
	@echo "Docker (full stack):"
	@echo "  docker-build     Build the Go app Docker image"
	@echo "  docker-up        Run app + PaddleOCR in Docker"
	@echo "  docker-up-all    Run app + PaddleOCR + Tesseract in Docker"
	@echo "  docker-down      Stop all Docker containers"
	@echo "  docker-run       One-shot translate (make docker-run INPUT=file.pdf TO=spanish)"
	@echo "  docker-logs      Tail all Docker logs"
	@echo ""
	@echo "Other:"
	@echo "  health           Check if OCR services are reachable"

.DEFAULT_GOAL := help
