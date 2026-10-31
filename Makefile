BIN_DIR := bin
CMD_DIR := cmd
PKG_DIR := pkg
FRAMES ?= 6500
PACK_SCALE ?= 4
PACK_PAGES ?= 1
BG_GROUP ?= 8
BG_PROTECT ?= 2
BOT ?= alex-kidd
SONIC_ALL ?=
CASTLE_ALL ?=
PREVIEW_FILTER ?= modern
PREVIEW_HD ?= false
PREVIEW_OUT ?= preview.png
CINE_ONLY ?=
CINE_MATCH ?=
CINE_LIMIT ?= 0
CINE_SKIP ?= 0
CINE_TILES ?= 1
PACK_STYLE ?= cinematic
CINE_BEVEL ?=
CINE_LIGHT ?=
CINE_NOISE ?=
CINE_SHADED ?= 0
CINE_HEIGHTS ?= 0
PERF_FRAMES ?= 3600
BENCH_WARMUP ?= 3600
BENCH_SCRIPT ?=
BENCH_FILTER ?= 5
BENCH_HD ?= 1
BENCH_SCALE ?= 5.625
BENCH_HD_FLAG := $(if $(filter 1,$(BENCH_HD)),-hd,)
BENCH_SCRIPT_FLAG := $(if $(BENCH_SCRIPT),-bench-script "$(BENCH_SCRIPT)",)

PY ?= $(shell [ -x .venv-remaster/bin/python ] && echo .venv-remaster/bin/python || echo python3)

EMU_BIN      := $(BIN_DIR)/emulator

BLUE    := \033[1;34m
CYAN    := \033[1;36m
GREEN   := \033[1;32m
YELLOW  := \033[1;33m
WHITE   := \033[1;37m
RESET   := \033[0m

.DEFAULT_GOAL := help
.PHONY: help build run clean objects remaster unpack cinematic pack preview setup test bench perf bench-ui bench-ui-matrix verify-hd audit-alpha select-hd check-music release release-complete audit android-docker clean-assets gemini-sheet gemini-slice

help:
	@printf "\n$(BLUE)  ╔══════════════════════════════════════════════════════════════╗$(RESET)\n"
	@printf "$(BLUE)  ║$(CYAN)      G O S M S   -   Sega Master System Emulator        $(BLUE)║$(RESET)\n"
	@printf "$(BLUE)  ╚══════════════════════════════════════════════════════════════╝$(RESET)\n\n"
	@printf "  $(GREEN)make run$(RESET)        Jugar ROM: $(CYAN)make run ROM=roms/sonic.sms$(RESET)\n"
	@printf "  $(GREEN)make test$(RESET)       Ejecuta todos los tests de Go\n"
	@printf "  $(GREEN)make bench$(RESET)      Microbenchmarks de CPU/VDP/PSG/máquina/vídeo (sin ROM externa)\n"
	@printf "  $(GREEN)make verify-hd$(RESET)  Comprueba contaminación de tiles en un pack HD: make verify-hd ROM=...\n"
	@printf "  $(GREEN)make audit-alpha$(RESET) Lista tiles que pierden o ganan transparencia: make audit-alpha ROM=...\n"
	@printf "  $(GREEN)make select-hd$(RESET)  Activa un estilo de pack: make select-hd ROM=... HD_STYLE=modern-plus|cinematic\n"
	@printf "  $(GREEN)make check-music$(RESET) Valida y convierte music/theme.ogg a Vorbis si hace falta\n"
	@printf "  $(GREEN)make perf$(RESET)       FPS de emulación headless: make perf ROM=... [PERF_FRAMES=3600]\n"
	@printf "  $(GREEN)make bench-ui$(RESET)   FPS real HD+Remaster 2026: make bench-ui ROM=... [BENCH_WARMUP=3600]\n"
	@printf "  $(GREEN)make bench-ui-matrix$(RESET) Matrix HD off/on × pixel/Remaster 2026\n"
	@printf "  $(GREEN)make objects$(RESET)    Limpia y extrae tiles, mapas y bloques de fondo (FRAMES=6500 BG_GROUP=8)\n"
	@printf "                              $(CYAN)SONIC_ALL=1$(RESET) recorre los 18 niveles de Sonic 1\n"
	@printf "                              $(CYAN)CASTLE_ALL=1$(RESET) recorre los 6 mundos de Castle of Illusion\n"
	@printf "  $(GREEN)make remaster$(RESET)  Borra, extrae, remasteriza en cinematográfico y empaqueta una ROM\n"
	@printf "  $(GREEN)make unpack$(RESET)     Prepara composites por objeto en remaster_workspace/\n"
	@printf "  $(GREEN)make cinematic$(RESET)  Remasteriza todo el catálogo en HD (PACK_STYLE=cinematic|modern-plus)\n"
	@printf "  $(GREEN)make pack$(RESET)       Empaqueta en páginas v2 (sin spritesheet) con la misma PACK_SCALE\n"
	@printf "  $(GREEN)make gemini-sheet$(RESET) Genera la hoja de 33 poses de Alex para Gemini\n"
	@printf "  $(GREEN)make gemini-slice$(RESET) Recorta la hoja devuelta por Gemini (SHEET=...)\n"
	@printf "  $(GREEN)make preview$(RESET)    Renderiza un frame: make preview ROM=... [PREVIEW_FILTER=modern] [PREVIEW_HD=true]\n"
	@printf "  $(GREEN)make setup$(RESET)      Crea .venv-remaster con numpy (opcional, acelera el vectorizado)\n"
	@printf "  $(GREEN)make release$(RESET)    Compila para Linux/Windows/Android y genera los zips\n"
	@printf "  $(GREEN)make release-complete$(RESET) Linux/Windows con todas las ROMs y packs HD (uso local)\n"
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

HD_STYLE ?= modern-plus

select-hd:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)"); ROM_BASE=$${ROM_BASE%.*}; \
	if [ ! -d "assets/$$ROM_BASE/hd-$(HD_STYLE)" ]; then \
		printf "No existe assets/$$ROM_BASE/hd-$(HD_STYLE)\n"; exit 2; \
	fi; \
	rm -rf "assets/$$ROM_BASE/hd"; \
	cp -a "assets/$$ROM_BASE/hd-$(HD_STYLE)" "assets/$$ROM_BASE/hd"; \
	printf "Pack HD activo: $(HD_STYLE)\n"
else
	@printf "Uso: make select-hd ROM='roms/juego.sms' HD_STYLE=modern-plus|cinematic\n"
	@exit 2
endif

audit-alpha:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)"); ROM_BASE=$${ROM_BASE%.*}; \
	$(PY) tools/audit_alpha.py "assets/$$ROM_BASE/hd" "assets/$$ROM_BASE/original"
else
	@printf "Uso: make audit-alpha ROM='roms/juego.sms'\n"
	@exit 2
endif

verify-hd:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)"); ROM_BASE=$${ROM_BASE%.*}; \
	$(PY) tools/verify_hd.py "assets/$$ROM_BASE/hd" "assets/$$ROM_BASE/original"
else
	@printf "Uso: make verify-hd ROM='roms/juego.sms'\n"
	@exit 2
endif

bench:
	@printf "$(BLUE)==> Microbenchmarks (3 pasadas, con memoria)...$(RESET)\n"
	@go test ./pkg/cpu ./pkg/vdp ./pkg/psg ./pkg/machine ./pkg/video -run '^$$' -bench . -benchmem -count 3

perf:
ifdef ROM
	@printf "$(BLUE)==> FPS de emulación headless (%s frames)...$(RESET)\n" "$(PERF_FRAMES)"
	@go run ./cmd/headless -bench -frames "$(PERF_FRAMES)" "$(ROM)"
else
	@printf "Uso: make perf ROM='roms/juego.sms' [PERF_FRAMES=3600]\n"
	@exit 2
endif

bench-ui:
ifdef ROM
	@printf "$(BLUE)==> FPS real con ventana (HD=$(BENCH_HD), filtro=$(BENCH_FILTER), escala=$(BENCH_SCALE), warmup=$(BENCH_WARMUP))...$(RESET)\n"
	@go run ./cmd/emulator -bench "$(PERF_FRAMES)" -bench-warmup "$(BENCH_WARMUP)" $(BENCH_SCRIPT_FLAG) -filter "$(BENCH_FILTER)" -scale "$(BENCH_SCALE)" $(BENCH_HD_FLAG) "$(ROM)"
else
	@printf "Uso: make bench-ui ROM='roms/juego.sms' [PERF_FRAMES=3600] [BENCH_WARMUP=3600] [BENCH_FILTER=5] [BENCH_HD=1] [BENCH_SCALE=5.625]\n"
	@exit 2
endif

bench-ui-matrix:
ifdef ROM
	@$(MAKE) bench-ui ROM="$(ROM)" BENCH_HD=0 BENCH_FILTER=0
	@$(MAKE) bench-ui ROM="$(ROM)" BENCH_HD=0 BENCH_FILTER=5
	@$(MAKE) bench-ui ROM="$(ROM)" BENCH_HD=1 BENCH_FILTER=0
	@$(MAKE) bench-ui ROM="$(ROM)" BENCH_HD=1 BENCH_FILTER=5
else
	@printf "Uso: make bench-ui-matrix ROM='roms/juego.sms' [PERF_FRAMES=3600] [BENCH_WARMUP=3600]\n"
	@exit 2
endif

check-music:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)"); ROM_BASE=$${ROM_BASE%.*}; \
	MUSIC="assets/$$ROM_BASE/music/theme.ogg"; \
	if [ ! -f "$$MUSIC" ]; then \
		printf "$(YELLOW)   Sin theme.ogg para %s (opcional)$(RESET)\n" "$$ROM_BASE"; \
		exit 0; \
	fi; \
	if ! command -v ffprobe >/dev/null 2>&1; then \
		printf "$(YELLOW)   theme.ogg presente; instala ffmpeg/ffprobe para validarlo$(RESET)\n"; \
		exit 0; \
	fi; \
	CODEC=$$(ffprobe -v error -select_streams a:0 -show_entries stream=codec_name -of default=nw=1:nk=1 "$$MUSIC"); \
	if [ "$$CODEC" = "vorbis" ]; then \
		printf "$(GREEN)   theme.ogg OK (Vorbis)$(RESET)\n"; \
	else \
		printf "$(BLUE)   theme.ogg es %s; convirtiendo a Vorbis...$(RESET)\n" "$$CODEC"; \
		if ! command -v ffmpeg >/dev/null 2>&1; then \
			printf "$(YELLOW)   Instala ffmpeg para convertir theme.ogg$(RESET)\n"; exit 1; \
		fi; \
		cp "$$MUSIC" "$$MUSIC.original.$$CODEC"; \
		ffmpeg -y -hide_banner -loglevel error -i "$$MUSIC" -c:a libvorbis -q:a 6 "$$MUSIC.tmp.ogg" || { rm -f "$$MUSIC.tmp.ogg"; exit 1; }; \
		mv "$$MUSIC.tmp.ogg" "$$MUSIC"; \
		printf "$(GREEN)   theme.ogg convertido a Vorbis (original: %s.original.%s)$(RESET)\n" "$$MUSIC" "$$CODEC"; \
	fi
else
	@printf "Uso: make check-music ROM='roms/juego.sms'\n"
	@exit 2
endif

clean-assets:
	@printf "$(YELLOW)==> Eliminando assets generados (se conserva music/)...$(RESET)\n"
	@if [ -d assets ]; then \
		find assets -mindepth 1 -maxdepth 1 -type d | while read dir; do \
			rm -rf "$$dir/original" "$$dir/hd" "$$dir/remaster_workspace" "$$dir/screenshots"; \
			for extra in "$$dir"/hd-*; do [ -e "$$extra" ] && rm -rf "$$extra"; done; \
		done; \
	fi

dump objects: clean-assets
ifdef ROM
	@$(MAKE) --no-print-directory check-music ROM="$(ROM)"
	@printf "$(BLUE)==> Extrayendo tiles, objetos y mapas de escena...$(RESET)\n"
	@go run ./cmd/dump_assets -frames "$(FRAMES)" -bot "$(BOT)" -bg-group "$(BG_GROUP)" -bg-protect "$(BG_PROTECT)" $(if $(SONIC_ALL),-sonic,) $(if $(CASTLE_ALL),-castle,) "$(ROM)"
else
	@printf "Uso: make objects ROM='roms/Alex Kidd in Miracle World.sms'\n"
	@exit 2
endif

remaster:
ifdef ROM
	@$(MAKE) objects ROM="$(ROM)"
	@$(MAKE) unpack ROM="$(ROM)"
	@$(MAKE) gemini-sheet ROM="$(ROM)"
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
	set -- --rom-name "$$ROM_BASE" --style "$(PACK_STYLE)" --limit "$(CINE_LIMIT)"; \
	if [ -n "$(CINE_BEVEL)" ]; then set -- "$$@" --bevel "$(CINE_BEVEL)"; fi; \
	if [ -n "$(CINE_LIGHT)" ]; then set -- "$$@" --light "$(CINE_LIGHT)"; fi; \
	if [ -n "$(CINE_NOISE)" ]; then set -- "$$@" --noise "$(CINE_NOISE)"; fi; \
	if [ -n "$(CINE_ONLY)" ]; then set -- "$$@" --only "$(CINE_ONLY)"; fi; \
	if [ -n "$(CINE_MATCH)" ]; then set -- "$$@" --match "$(CINE_MATCH)"; fi; \
	if [ "$(CINE_SKIP)" = "1" ]; then set -- "$$@" --skip-existing; fi; \
	if [ "$(CINE_TILES)" = "0" ]; then set -- "$$@" --no-tiles; fi; \
	if [ "$(CINE_SHADED)" = "1" ]; then set -- "$$@" --shaded; fi; \
	if [ "$(CINE_HEIGHTS)" = "0" ]; then set -- "$$@" --no-heights; fi; \
	GOSMS_SCALE=$(PACK_SCALE) PYTHONUNBUFFERED=1 $(PY) tools/cinematic_tool.py "$$@"
else
	@printf "Uso: make cinematic ROM='roms/juego.sms' [PACK_SCALE=4] [PACK_STYLE=cinematic|modern-plus] [CINE_MATCH=sprite_] [CINE_ONLY=id] [CINE_LIMIT=0] [CINE_SKIP=0]\n"
	@exit 2
endif

gemini-sheet:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)"); ROM_BASE=$${ROM_BASE%.*}; \
	printf "$(BLUE)==> Generando hoja de poses para Gemini...$(RESET)\n"; \
	$(PY) tools/pose_sheet.py build "$$ROM_BASE"
else
	@printf "Uso: make gemini-sheet ROM='roms/juego.sms'\n"
	@exit 2
endif

gemini-slice:
ifdef ROM
	@test -n "$(SHEET)" || (printf "Uso: make gemini-slice ROM='roms/juego.sms' SHEET=ruta/poses_sheet_gemini.png [PACK_AI=1]\n"; exit 2)
	@ROM_BASE=$$(basename "$(ROM)"); ROM_BASE=$${ROM_BASE%.*}; \
	printf "$(BLUE)==> Recortando hoja de Gemini...$(RESET)\n"; \
	$(PY) tools/pose_sheet.py slice "$$ROM_BASE" "$(SHEET)" $(if $(PACK_AI),--pack,)
else
	@printf "Uso: make gemini-slice ROM='roms/juego.sms' SHEET=ruta/poses_sheet_gemini.png [PACK_AI=1]\n"
	@exit 2
endif

pack:
ifdef ROM
	@ROM_BASE=$$(basename "$(ROM)"); ROM_BASE=$${ROM_BASE%.*}; \
	printf "$(BLUE)==> Empaquetando workspace a HD %sx...$(RESET)\n" "$(PACK_SCALE)"; \
	GOSMS_SCALE=$(PACK_SCALE) GOSMS_PAGES=$(PACK_PAGES) $(PY) tools/tile_tool.py pack "$$ROM_BASE"
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
	@cp README.md CONTROLES.md dist/gosms-linux/
	@if [ -d "assets" ]; then cp -r assets dist/gosms-linux/; fi
	@mkdir -p dist/gosms-linux/roms
	@echo "Coloca tus ROMs .sms o .bin aquí" > dist/gosms-linux/roms/leer_roms.txt
	@cd dist && rm -f gosms-linux-release.zip && zip -r gosms-linux-release.zip gosms-linux > /dev/null
	@printf "$(BLUE)==> Preparando paquete portable para Windows...$(RESET)\n"
	@cp bin/gosms-win64.exe dist/gosms-windows/gosms.exe
	@cp README.md CONTROLES.md dist/gosms-windows/
	@if [ -d "assets" ]; then cp -r assets dist/gosms-windows/; fi
	@mkdir -p dist/gosms-windows/roms
	@echo "Coloca tus ROMs .sms o .bin aquí" > dist/gosms-windows/roms/leer_roms.txt
	@cd dist && rm -f gosms-windows-release.zip && zip -r gosms-windows-release.zip gosms-windows > /dev/null
	@$(MAKE) android-docker
	@printf "$(GREEN)✨ ¡BUILDS DE RELEASE COMPLETADOS!$(RESET)\n"
	@printf "  🐧 Linux ZIP:   $(CYAN)dist/gosms-linux-release.zip$(RESET)\n"
	@printf "  🪟 Windows ZIP: $(CYAN)dist/gosms-windows-release.zip$(RESET)\n"
	@printf "  📱 Android APK: $(CYAN)dist/gosms-android/gosms.apk$(RESET)\n"

# Release local con contenido sensible al copyright: ROMs y packs HD.
# Los ZIP resultantes NO deben subirse a GitHub ni a ningún sitio público.
release-complete:
	@mkdir -p bin dist
	@printf "$(BLUE)==> Compilando para Linux (Ubuntu)...$(RESET)\n"
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/gosms-linux ./$(CMD_DIR)/emulator
	@printf "$(BLUE)==> Compilando para Windows (Win64 sin consola)...$(RESET)\n"
	@GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui -s -w" -o bin/gosms-win64.exe ./$(CMD_DIR)/emulator
	@set -e; \
	for pair in "linux:gosms-linux:gosms" "windows:gosms-win64.exe:gosms.exe"; do \
		os=$${pair%%:*}; rest=$${pair#*:}; bin=$${rest%%:*}; exe=$${rest#*:}; \
		dir="dist/gosms-$$os-complete"; \
		printf "$(BLUE)==> Preparando paquete completo para %s...$(RESET)\n" "$$os"; \
		rm -rf "$$dir"; \
		mkdir -p "$$dir/roms"; \
		cp "bin/$$bin" "$$dir/$$exe"; \
		cp README.md CONTROLES.md "$$dir/"; \
		if [ -d roms ]; then \
			find roms -maxdepth 1 -type f \( -name '*.sms' -o -name '*.bin' -o -name '*.sg' \) -exec cp {} "$$dir/roms/" \;; \
		fi; \
		rom_count=$$(find "$$dir/roms" -type f | wc -l | tr -d ' '); \
		for hd in assets/*/hd; do \
			[ -f "$$hd/manifest.json" ] || [ -f "$$hd/pack.json" ] || continue; \
			game=$$(basename "$$(dirname "$$hd")"); \
			mkdir -p "$$dir/assets/$$game"; \
			cp -r "$$hd" "$$dir/assets/$$game/"; \
		done; \
		for music in assets/*/music; do \
			[ -d "$$music" ] || continue; \
			game=$$(basename "$$(dirname "$$music")"); \
			mkdir -p "$$dir/assets/$$game"; \
			cp -r "$$music" "$$dir/assets/$$game/"; \
		done; \
		printf "GoSMS completo para %s\n\n" "$$os" > "$$dir/LEEME.txt"; \
		printf "Incluye el emulador, %s ROMs en roms/ y los packs HD de assets/<juego>/hd/.\n\n" "$$rom_count" >> "$$dir/LEEME.txt"; \
		printf "Como jugar:\n- Linux: ./gosms (si hace falta: chmod +x gosms)\n- Windows: gosms.exe\n\n" >> "$$dir/LEEME.txt"; \
		printf "Controles:\n- Flechas: mover. Z/Espacio: boton 1. X: boton 2. P/Enter: pausa.\n- F10: HD/original. F1: filtros (Remaster 2026, Modern+, Modern+ Parallax).\n- F2: audio. F6/M: musica. F11: pantalla completa. F12/H: ayuda.\n- F3/F4: guardar/cargar. F5: captura. Esc: menu.\n\n" >> "$$dir/LEEME.txt"; \
		printf "Notas:\n- Los packs HD son de Alex Kidd y Castle of Illusion; el resto se juega en original.\n- Alex Kidd incluye music/theme.ogg: activala con F6/M.\n- Material con copyright para uso local; no lo publiques.\n" >> "$$dir/LEEME.txt"; \
		(cd dist && rm -f "gosms-$$os-complete.zip" && zip -qr "gosms-$$os-complete.zip" "gosms-$$os-complete"); \
	done
	@printf "$(GREEN)✨ RELEASES COMPLETAS LOCALES LISTAS$(RESET)\n"
	@printf "  🐧 Linux:   $(CYAN)dist/gosms-linux-complete.zip$(RESET) ($$(du -h dist/gosms-linux-complete.zip | cut -f1))\n"
	@printf "  🪟 Windows: $(CYAN)dist/gosms-windows-complete.zip$(RESET) ($$(du -h dist/gosms-windows-complete.zip | cut -f1))\n"
	@printf "  $(YELLOW)⚠️  No subas estos ZIP a GitHub: incluyen ROMs y assets con copyright.$(RESET)\n"

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
