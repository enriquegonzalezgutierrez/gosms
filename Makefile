BIN_DIR := bin
CMD_DIR := cmd
PKG_DIR := pkg

EMU_BIN      := $(BIN_DIR)/emulator
HEADLESS_BIN := $(BIN_DIR)/headless
TRACER_BIN   := $(BIN_DIR)/tracer
UPSCALER_DIR := tools/upscaler
UPSCALER_BIN := $(UPSCALER_DIR)/waifu2x-ncnn-vulkan

BLUE    := \033[1;34m
CYAN    := \033[1;36m
GREEN   := \033[1;32m
YELLOW  := \033[1;33m
WHITE   := \033[1;37m
RESET   := \033[0m

.DEFAULT_GOAL := help

.PHONY: help build run menu test fmt vet clean hd

help:
	@printf "\n$(BLUE)  ╔══════════════════════════════════════════════════════════════╗$(RESET)\n"
	@printf "$(BLUE)  ║$(CYAN)      G O S M S   -   Sega Master System Emulator        $(BLUE)║$(RESET)\n"
	@printf "$(BLUE)  ║$(WHITE)      Desarrollado por Enrique González Gutiérrez        $(BLUE)║$(RESET)\n"
	@printf "$(BLUE)  ╚══════════════════════════════════════════════════════════════╝$(RESET)\n\n"
	@printf "  $(YELLOW)Uso:$(RESET) make $(CYAN)<comando>$(RESET) [ROM=ruta/juego.sms]\n\n"
	@printf "  $(YELLOW)Comandos principales:$(RESET)\n"
	@printf "    $(GREEN)make run$(RESET)        Juega y captura texturas limpias: $(CYAN)make run ROM=roms/sonic.sms$(RESET)\n"
	@printf "    $(GREEN)make hd$(RESET)         Convierte todos los tiles capturados a 4K con IA en lote\n"
	@printf "    $(GREEN)make build$(RESET)      Compila el emulador\n"
	@printf "    $(GREEN)make clean$(RESET)      Limpia binarios\n\n"

build:
	@mkdir -p $(BIN_DIR)
	@go build -ldflags="-s -w" -o $(EMU_BIN) ./$(CMD_DIR)/emulator
	@go build -ldflags="-s -w" -o $(HEADLESS_BIN) ./$(CMD_DIR)/headless
	@go build -ldflags="-s -w" -o $(TRACER_BIN) ./$(CMD_DIR)/tracer

run: build
ifdef ROM
	@./$(EMU_BIN) -crop "$(ROM)"
else
	@./$(EMU_BIN)
endif

## hd: Convierte en lote todas las texturas de original/ a hd/ en 4K por IA
hd:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)" .sms); \
	ORIG_DIR="assets/$$ROM_BASE/original/tiles"; \
	HD_DIR="assets/$$ROM_BASE/hd/tiles"; \
	if [ ! -d "$$ORIG_DIR" ] || [ -z "$$(ls -A "$$ORIG_DIR" 2>/dev/null)" ]; then \
		echo "Error: No hay tiles en $$ORIG_DIR. Juega una partida primero con 'make run' para capturarlos."; \
		exit 1; \
	fi; \
	printf "$(BLUE)==> Procesando tiles capturados con IA a 4K (Vulkan GPU)...$(RESET)\n"; \
	$(UPSCALER_BIN) -i "$$ORIG_DIR" -o "$$HD_DIR" -s 4 -n 2 -m $(UPSCALER_DIR)/models-cunet; \
	COUNT=$$(ls -1 "$$HD_DIR" 2>/dev/null | wc -l); \
	printf "$(GREEN)✨ PACK 4K ACTUALIZADO: $$COUNT texturas listas para jugar en HD.$(RESET)\n"
else
	@echo "Error: Debes especificar la ROM. Ejemplo: make hd ROM=\"roms/Sonic The Hedgehog (USA, Europe).sms\""
endif

clean:
	@rm -rf $(BIN_DIR) frame.png

test:
	@go test -v ./...
