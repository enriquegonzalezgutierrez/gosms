BIN_DIR := bin
CMD_DIR := cmd
PKG_DIR := pkg
FRAMES ?= 6500
PACK_SCALE ?= 4
BG_GROUP ?= 8
BG_PROTECT ?= 2
BOT ?= alex-kidd
PREVIEW_FILTER ?= modern
PREVIEW_HD ?= false
PREVIEW_OUT ?= preview.png
CINE_ONLY ?=
CINE_MATCH ?=
CINE_LIMIT ?= 0
CINE_SKIP ?= 0
CINE_TILES ?= 1
CINE_BEVEL ?= 1.8
CINE_LIGHT ?= 315,48
CINE_NOISE ?= 0.03
CINE_SHADED ?= 0
CINE_HEIGHTS ?= 1

PY ?= $(shell [ -x .venv-remaster/bin/python ] && echo .venv-remaster/bin/python || echo python3)

EMU_BIN      := $(BIN_DIR)/emulator

BLUE    := \033[1;34m
CYAN    := \033[1;36m
GREEN   := \033[1;32m
YELLOW  := \033[1;33m
WHITE   := \033[1;37m
RESET   := \033[0m

.DEFAULT_GOAL := help
.PHONY: help build run clean objects remaster unpack cinematic pack preview setup test release audit android-docker clean-assets

help:
	@printf "\n$(BLUE)  ╔══════════════════════════════════════════════════════════════╗$(RESET)\n"
	@printf "$(BLUE)  ║$(CYAN)      G O S M S   -   Sega Master System Emulator        $(BLUE)║$(RESET)\n"
	@printf "$(BLUE)  ╚══════════════════════════════════════════════════════════════╝$(RESET)\n\n"
	@printf "  $(GREEN)make run$(RESET)        Jugar ROM: $(CYAN)make run ROM=roms/sonic.sms$(RESET)\n"
	@printf "  $(GREEN)make test$(RESET)       Ejecuta todos los tests de Go\n"
	@printf "  $(GREEN)make objects$(RESET)    Limpia y extrae tiles, mapas y bloques de fondo (FRAMES=6500 BG_GROUP=8)\n"
	@printf "  $(GREEN)make remaster$(RESET)  Borra, extrae, remasteriza en cinematográfico y empaqueta una ROM\n"
	@printf "  $(GREEN)make unpack$(RESET)     Prepara composites por objeto en remaster_workspace/\n"
	@printf "  $(GREEN)make cinematic$(RESET)  Remasteriza todo el catálogo en HD cinematográfico (sin IA)\n"
	@printf "  $(GREEN)make pack$(RESET)       Empaqueta los objetos remasterizados (misma PACK_SCALE que el vectorizado)\n"
	@printf "  $(GREEN)make preview$(RESET)    Renderiza un frame: make preview ROM=... [PREVIEW_FILTER=modern] [PREVIEW_HD=true]\n"
	@printf "  $(GREEN)make setup$(RESET)      Crea .venv-remaster con numpy (opcional, acelera el vectorizado)\n"
	@printf "  $(GREEN)make release$(RESET)    Compila para Linux/Windows/Android y genera los zips\n"
	@printf "  $(GREEN)make audit$(RESET)      Ejecuta auditoría de compatibilidad de todas las ROMs\n"
	@printf "  $(GREEN)make clean$(RESET)      Limpia los binarios compilados\n\n"

build:
	@mkdir -p $(BIN_DIR)
	@go build -ldflags="-s -w" -o $(EMU_BIN) ./$(CMD_DIR)/emulator

run: build
ifdef ROM
	@./$(EMU_BIN) -crop "$(ROM)"
else
	@./$(EMU_BIN)
endif

test:
	@go test ./...

clean-assets:
	@printf "$(YELLOW)==> Eliminando la carpeta assets/ completa...$(RESET)\n"
	@rm -rf assets

dump objects: clean-assets
ifdef ROM
	@printf "$(BLUE)==> Extrayendo tiles, objetos y mapas de escena...$(RESET)\n"
	@go run ./cmd/dump_assets -frames "$(FRAMES)" -bot "$(BOT)" -bg-group "$(BG_GROUP)" -bg-protect "$(BG_PROTECT)" "$(ROM)"
else
	@printf "Uso: make objects ROM='roms/Alex Kidd in Miracle World.sms'\n"
	@exit 2
endif

remaster:
ifdef ROM
	@$(MAKE) objects ROM="$(ROM)"
	@$(MAKE) unpack ROM="$(ROM)"
	@$(MAKE) cinematic ROM="$(ROM)"
	@$(MAKE) pack ROM="$(ROM)"
else
	@printf "Uso: make remaster ROM='roms/Alex Kidd in Miracle World.sms' [FRAMES=6500]\n"
	@exit 2
endif

unpack:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)"); ROM_BASE=$${ROM_BASE%.*}; \
	printf "$(BLUE)==> Desempaquetando tiles al workspace...$(RESET)\n"; \
	$(PY) tools/tile_tool.py unpack "$$ROM_BASE"
else
	@printf "Uso: make unpack ROM='roms/Alex Kidd in Miracle World.sms'\n"
	@exit 2
endif

setup:
	@printf "$(BLUE)==> Creando .venv-remaster con numpy...$(RESET)\n"
	@python3 -m venv .venv-remaster
	@.venv-remaster/bin/pip install --upgrade pip > /dev/null
	@.venv-remaster/bin/pip install numpy Pillow

cinematic:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)"); ROM_BASE=$${ROM_BASE%.*}; \
	set -- --rom-name "$$ROM_BASE" --limit "$(CINE_LIMIT)" --bevel "$(CINE_BEVEL)" \
		--light "$(CINE_LIGHT)" --noise "$(CINE_NOISE)"; \
	if [ -n "$(CINE_ONLY)" ]; then set -- "$$@" --only "$(CINE_ONLY)"; fi; \
	if [ -n "$(CINE_MATCH)" ]; then set -- "$$@" --match "$(CINE_MATCH)"; fi; \
	if [ "$(CINE_SKIP)" = "1" ]; then set -- "$$@" --skip-existing; fi; \
	if [ "$(CINE_TILES)" = "0" ]; then set -- "$$@" --no-tiles; fi; \
	if [ "$(CINE_SHADED)" = "1" ]; then set -- "$$@" --shaded; fi; \
	if [ "$(CINE_HEIGHTS)" = "0" ]; then set -- "$$@" --no-heights; fi; \
	GOSMS_SCALE=$(PACK_SCALE) PYTHONUNBUFFERED=1 $(PY) tools/cinematic_tool.py "$$@"
else
	@printf "Uso: make cinematic ROM='roms/juego.sms' [PACK_SCALE=4] [CINE_MATCH=sprite_] [CINE_ONLY=id] [CINE_LIMIT=0] [CINE_SKIP=0]\n"
	@exit 2
endif

pack:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)"); ROM_BASE=$${ROM_BASE%.*}; \
	printf "$(BLUE)==> Empaquetando workspace a HD %sx...$(RESET)\n" "$(PACK_SCALE)"; \
	GOSMS_SCALE=$(PACK_SCALE) $(PY) tools/tile_tool.py pack "$$ROM_BASE"
else
	@printf "Uso: make pack ROM='roms/Alex Kidd in Miracle World.sms'\n"
	@exit 2
endif

preview:
ifdef ROM
	@printf "$(BLUE)==> Renderizando preview (%s, HD=%s)...$(RESET)\n" "$(PREVIEW_FILTER)" "$(PREVIEW_HD)"
	@go run ./cmd/preview -frames "$(FRAMES)" -filter "$(PREVIEW_FILTER)" -hd=$(PREVIEW_HD) -out "$(PREVIEW_OUT)" "$(ROM)"
else
	@printf "Uso: make preview ROM='roms/juego.sms' [PREVIEW_FILTER=pixel|smooth|modern|crt|scanlines|mono] [PREVIEW_HD=true] [PREVIEW_OUT=preview.png]\n"
	@exit 2
endif

audit:
	@printf "$(BLUE)==> Ejecutando auditoría general de ROMs...$(RESET)\n"
	@go run ./cmd/audit

release:
	@mkdir -p bin dist/gosms-linux dist/gosms-windows
	@printf "$(BLUE)==> Compilando para Linux (Ubuntu)...$(RESET)\n"
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/gosms-linux ./$(CMD_DIR)/emulator
	@printf "$(BLUE)==> Compilando para Windows (Win64 sin consola)...$(RESET)\n"
	@GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui -s -w" -o bin/gosms-win64.exe ./$(CMD_DIR)/emulator
	@printf "$(BLUE)==> Preparando paquete portable para Linux...$(RESET)\n"
	@cp bin/gosms-linux dist/gosms-linux/gosms
	@if [ -d "assets" ]; then cp -r assets dist/gosms-linux/; fi
	@mkdir -p dist/gosms-linux/roms
	@echo "Coloca tus ROMs .sms o .bin aquí" > dist/gosms-linux/roms/leer_roms.txt
	@cd dist && rm -f gosms-linux-release.zip && zip -r gosms-linux-release.zip gosms-linux > /dev/null
	@printf "$(BLUE)==> Preparando paquete portable para Windows...$(RESET)\n"
	@cp bin/gosms-win64.exe dist/gosms-windows/gosms.exe
	@if [ -d "assets" ]; then cp -r assets dist/gosms-windows/; fi
	@mkdir -p dist/gosms-windows/roms
	@echo "Coloca tus ROMs .sms o .bin aquí" > dist/gosms-windows/roms/leer_roms.txt
	@cd dist && rm -f gosms-windows-release.zip && zip -r gosms-windows-release.zip gosms-windows > /dev/null
	@$(MAKE) android-docker
	@printf "$(GREEN)✨ ¡BUILDS DE RELEASE COMPLETADOS!$(RESET)\n"
	@printf "  🐧 Linux ZIP:   $(CYAN)dist/gosms-linux-release.zip$(RESET)\n"
	@printf "  🪟 Windows ZIP: $(CYAN)dist/gosms-windows-release.zip$(RESET)\n"
	@printf "  📱 Android APK: $(CYAN)dist/gosms-android/gosms.apk$(RESET)\n"

android-docker:
	@printf "$(BLUE)==> Verificando imagen Docker para Android...$(RESET)\n"
	@docker build -t gosms-android-builder -f Dockerfile.android . > /dev/null
	@mkdir -p dist/gosms-android
	@printf "$(BLUE)==> Compilando biblioteca nativa AAR (ARM, ARM64, x86)...$(RESET)\n"
	@docker run --rm -e GOFLAGS="-buildvcs=false" -v $$(pwd):/app -w /app gosms-android-builder ebitenmobile bind -target android -javapkg com.egonzalez.gosms -o dist/gosms-android/gosms.aar ./mobile
	@printf "$(BLUE)==> Ensamblando, optimizando y firmando gosms.apk...$(RESET)\n"
	@docker run --rm -v $$(pwd):/app -w /app gosms-android-builder ./scripts/build_apk.sh

clean:
	@rm -rf $(BIN_DIR) dist
