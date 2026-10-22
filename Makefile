# --- Variables ---
BIN_DIR := bin
CMD_DIR := cmd
PKG_DIR := pkg

# Nombres de los ejecutables
EMU_BIN     := $(BIN_DIR)/emulator
HEADLESS_BIN:= $(BIN_DIR)/headless
TRACER_BIN  := $(BIN_DIR)/tracer

# ROM por defecto si no se especifica otra
ROM ?= roms/alex_kidd.sms

# --- Comandos principales ---
.PHONY: all build run clean test fmt vet help

all: build

## build: Compila todos los ejecutables (emulator, headless, tracer)
build:
	@echo "==> Compilando binarios..."
	@mkdir -p $(BIN_DIR)
	go build -ldflags="-s -w" -o $(EMU_BIN) ./$(CMD_DIR)/emulator
	go build -ldflags="-s -w" -o $(HEADLESS_BIN) ./$(CMD_DIR)/headless
	go build -ldflags="-s -w" -o $(TRACER_BIN) ./$(CMD_DIR)/tracer
	@echo "==> Listo. Ejecutables en $(BIN_DIR)/"

## run: Ejecuta el emulador (ej: make run ROM=roms/sonic.sms)
run: build
	@echo "==> Ejecutando $(ROM)..."
	./$(EMU_BIN) -crop $(ROM)

## clean: Borra los binarios y archivos temporales
clean:
	@echo "==> Limpiando..."
	rm -rf $(BIN_DIR) frame.png
	@echo "==> Limpio."

## test: Ejecuta las pruebas unitarias
test:
	@echo "==> Ejecutando tests..."
	go test -v ./...

## fmt: Formatea el código fuente
fmt:
	@echo "==> Formateando código..."
	gofmt -w $(CMD_DIR) $(PKG_DIR)

## vet: Analiza el código en busca de posibles errores
vet:
	@echo "==> Analizando código con go vet..."
	go vet ./...

## help: Muestra esta ayuda
help:
	@echo "Uso:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'
