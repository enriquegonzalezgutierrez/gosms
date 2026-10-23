# --- Variables de Directorio ---
BIN_DIR := bin
CMD_DIR := cmd
PKG_DIR := pkg

# Nombres de los ejecutables
EMU_BIN      := $(BIN_DIR)/emulator
HEADLESS_BIN := $(BIN_DIR)/headless
TRACER_BIN   := $(BIN_DIR)/tracer

# Paleta de Colores ANSI
BLUE    := \033[1;34m
CYAN    := \033[1;36m
GREEN   := \033[1;32m
YELLOW  := \033[1;33m
WHITE   := \033[1;37m
RESET   := \033[0m

# Por defecto, ejecutar 'make' muestra la ayuda
.DEFAULT_GOAL := help

.PHONY: help build run menu test fmt vet clean

## help: Muestra esta ayuda interactiva en color
help:
	@printf "\n$(BLUE)  ╔══════════════════════════════════════════════════════════════╗$(RESET)\n"
	@printf "$(BLUE)  ║$(CYAN)      G O S M S   -   Sega Master System Emulator        $(BLUE)║$(RESET)\n"
	@printf "$(BLUE)  ║$(WHITE)      Desarrollado por Enrique González Gutiérrez        $(BLUE)║$(RESET)\n"
	@printf "$(BLUE)  ╚══════════════════════════════════════════════════════════════╝$(RESET)\n\n"
	@printf "  $(YELLOW)Uso:$(RESET) make $(CYAN)<comando>$(RESET) [ROM=ruta/juego.sms]\n\n"
	@printf "  $(YELLOW)Comandos principales:$(RESET)\n"
	@printf "    $(GREEN)make run$(RESET)        Abre el $(CYAN)lanzador gráfico$(RESET) con todos los juegos\n"
	@printf "                    $(WHITE)o una ROM directa:$(RESET) $(CYAN)make run ROM=roms/sonic.sms$(RESET)\n"
	@printf "    $(GREEN)make menu$(RESET)       Alias directo para abrir el lanzador de juegos\n"
	@printf "    $(GREEN)make build$(RESET)      Compila todos los ejecutables en $(CYAN)$(BIN_DIR)/$(RESET)\n"
	@printf "    $(GREEN)make test$(RESET)       Ejecuta el $(CYAN)100%% de los tests unitarios$(RESET)\n"
	@printf "    $(GREEN)make fmt$(RESET)        Aplica $(CYAN)gofmt$(RESET) a todo el código fuente\n"
	@printf "    $(GREEN)make vet$(RESET)        Analiza el código con $(CYAN)go vet$(RESET)\n"
	@printf "    $(GREEN)make clean$(RESET)      Elimina binarios y archivos temporales\n"
	@printf "    $(GREEN)make dump$(RESET)          Extrae todos los assets gráficos de una ROM\n"
	@printf "    $(GREEN)make audit$(RESET)      Audita la compatibilidad de las 333 ROMs\n\n"

## build: Compila todos los ejecutables (emulator, headless, tracer)
build:
	@printf "$(BLUE)==>$(RESET) Compilando binarios...\n"
	@mkdir -p $(BIN_DIR)
	@go build -ldflags="-s -w" -o $(EMU_BIN) ./$(CMD_DIR)/emulator
	@go build -ldflags="-s -w" -o $(HEADLESS_BIN) ./$(CMD_DIR)/headless
	@go build -ldflags="-s -w" -o $(TRACER_BIN) ./$(CMD_DIR)/tracer
	@printf "$(GREEN)==> Listo.$(RESET) Ejecutables listos en $(CYAN)$(BIN_DIR)/$(RESET)\n"

## menu: Alias para abrir la interfaz con todos los juegos
menu: run

## test: Ejecuta toda la batería de tests unitarios
test:
	@printf "$(BLUE)==>$(RESET) Ejecutando tests unitarios...\n"
	@go test -v ./...

## fmt: Formatea el código de todos los paquetes
fmt:
	@printf "$(BLUE)==>$(RESET) Formateando código con gofmt...\n"
	@gofmt -w $(CMD_DIR) $(PKG_DIR)

## vet: Análisis estático de código
vet:
	@printf "$(BLUE)==>$(RESET) Analizando código con go vet...\n"
	@go vet ./...

## clean: Borra ejecutables y temporales
clean:
	@printf "$(BLUE)==>$(RESET) Limpiando archivos compilados...\n"
	@rm -rf $(BIN_DIR) frame.png
	@printf "$(GREEN)==> Limpio.$(RESET)\n"

## audit: Analiza todas las ROMs e informa de fallos y compatibilidad
audit:
	@go run tools/audit.go


## run: Abre el lanzador de juegos o una ROM directa
run: build
ifdef ROM
	@./$(EMU_BIN) -crop "$(ROM)"
else
	@./$(EMU_BIN)
endif

## dump: Extrae todos los tiles y spritesheets de una ROM
dump:
ifdef ROM
	@go run tools/dump_assets.go "$(ROM)"
else
	@echo "Error: Debes especificar una ROM. Ejemplo: make dump ROM=\"roms/Sonic.sms\""
endif
