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
.PHONY: help build run hd clean dump

help:
	@printf "\n$(BLUE)  ╔══════════════════════════════════════════════════════════════╗$(RESET)\n"
	@printf "$(BLUE)  ║$(CYAN)      G O S M S   -   Sega Master System Emulator        $(BLUE)║$(RESET)\n"
	@printf "$(BLUE)  ╚══════════════════════════════════════════════════════════════╝$(RESET)\n\n"
	@printf "  $(GREEN)make run$(RESET)        Jugar ROM: $(CYAN)make run ROM=roms/sonic.sms$(RESET)\n"
	@printf "  $(GREEN)make dump$(RESET)       Extrae el 100%% del juego a spritesheet original\n"
	@printf "  $(GREEN)make hd$(RESET)         Pasa el spritesheet original por IA a 4K\n\n"

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
	$(UPSCALER_BIN) -i "$$ORIG_FILE" -o "$$HD_FILE" -s 4 -n 2 -m $(UPSCALER_DIR)/models-cunet; \
	printf "$(GREEN)✨ SPRITESHEET 4K GENERADO: $$HD_FILE$(RESET)\n"
endif

clean:
	@rm -rf $(BIN_DIR)
