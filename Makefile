BIN_DIR := bin
CMD_DIR := cmd
PKG_DIR := pkg

EMU_BIN      := $(BIN_DIR)/emulator
UPSCALER_DIR := tools/upscaler
UPSCALER_BIN := $(UPSCALER_DIR)/waifu2x-ncnn-vulkan

BLUE    := \033[1;34m
CYAN    := \033[1;36m
GREEN   := \033[1;32m
YELLOW  := \033[1;33m
WHITE   := \033[1;37m
RESET   := \033[0m

.DEFAULT_GOAL := help
.PHONY: help build run hd clean dump unpack pack release audit

help:
	@printf "\n$(BLUE)  ╔══════════════════════════════════════════════════════════════╗$(RESET)\n"
	@printf "$(BLUE)  ║$(CYAN)      G O S M S   -   Sega Master System Emulator        $(BLUE)║$(RESET)\n"
	@printf "$(BLUE)  ╚══════════════════════════════════════════════════════════════╝$(RESET)\n\n"
	@printf "  $(GREEN)make run$(RESET)        Jugar ROM: $(CYAN)make run ROM=roms/sonic.sms$(RESET)\n"
	@printf "  $(GREEN)make dump$(RESET)       Extrae el 100%% del juego a spritesheet original\n"
	@printf "  $(GREEN)make hd$(RESET)         Pasa el spritesheet original por IA a 4K (waifu2x)\n"
	@printf "  $(GREEN)make unpack$(RESET)     Separa el spritesheet en tiles 8x8 individuales para IA\n"
	@printf "  $(GREEN)make pack$(RESET)       Ensambla los tiles 32x32 procesados en un nuevo spritesheet 4K\n"
	@printf "  $(GREEN)make release$(RESET)    Compila para Linux/Windows y genera los .zip portables\n"
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

dump:
ifdef ROM
	@printf "$(BLUE)==> Extrayendo spritesheet...$(RESET)\n"
	@go run tools/dump_assets.go "$(ROM)"
endif

hd:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)" .sms); \
	ORIG_FILE="assets/$$ROM_BASE/original/spritesheet.png"; \
	HD_FILE="assets/$$ROM_BASE/hd/spritesheet.png"; \
	printf "$(BLUE)==> Reescalando Spritesheet Único a 4K...$(RESET)\n"; \
	$(UPSCALER_BIN) -i "$$ORIG_FILE" -o "$$HD_FILE" -s 4 -n 0 -m $(UPSCALER_DIR)/models-cunet; \
	printf "$(GREEN)✨ SPRITESHEET 4K GENERADO: $$HD_FILE$(RESET)\n"
endif

unpack:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)" .sms); \
	printf "$(BLUE)==> Desempaquetando tiles al workspace...$(RESET)\n"; \
	python3 tools/tile_tool.py unpack "$$ROM_BASE"
endif

pack:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)" .sms); \
	printf "$(BLUE)==> Empaquetando workspace a HD 4K...$(RESET)\n"; \
	python3 tools/tile_tool.py pack "$$ROM_BASE"
endif

audit:
	@printf "$(BLUE)==> Ejecutando auditoría general de ROMs...$(RESET)\n"
	@go run tools/audit.go

release:
	@mkdir -p bin dist/gosms-linux dist/gosms-windows
	@printf "$(BLUE)==> Compilando para Linux (Ubuntu)...\033[0m\n"
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/gosms-linux ./$(CMD_DIR)/emulator
	@printf "$(BLUE)==> Compilando para Windows (Win64 sin consola)...\033[0m\n"
	@GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui -s -w" -o bin/gosms-win64.exe ./$(CMD_DIR)/emulator
	@printf "$(BLUE)==> Preparando paquete portable para Linux...\033[0m\n"
	@cp bin/gosms-linux dist/gosms-linux/gosms
	@if [ -d "assets" ]; then cp -r assets dist/gosms-linux/; fi
	@mkdir -p dist/gosms-linux/roms
	@echo "Coloca tus ROMs .sms o .bin aquí" > dist/gosms-linux/roms/leer_roms.txt
	@cd dist && rm -f gosms-linux-release.zip && zip -r gosms-linux-release.zip gosms-linux > /dev/null
	@printf "$(BLUE)==> Preparando paquete portable para Windows...\033[0m\n"
	@cp bin/gosms-win64.exe dist/gosms-windows/gosms.exe
	@if [ -d "assets" ]; then cp -r assets dist/gosms-windows/; fi
	@mkdir -p dist/gosms-windows/roms
	@echo "Coloca tus ROMs .sms o .bin aquí" > dist/gosms-windows/roms/leer_roms.txt
	@cd dist && rm -f gosms-windows-release.zip && zip -r gosms-windows-release.zip gosms-windows > /dev/null
	@printf "$(GREEN)✨ ¡BUILDS DE RELEASE COMPLETADOS!\033[0m\n"
	@printf "  🐧 Linux ZIP:   $(CYAN)dist/gosms-linux-release.zip\033[0m\n"
	@printf "  🪟 Windows ZIP: $(CYAN)dist/gosms-windows-release.zip\033[0m\n"

clean:
	@rm -rf $(BIN_DIR) dist
