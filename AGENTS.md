# Repository Guidelines

## Project Structure & Module Organization

GoSMS is a Sega Master System emulator written in Go (module `gosms`). Core emulation code lives in `pkg/`: `cpu`, `vdp`, `psg`, and `machine`. Executable entry points are under `cmd/`, including `emulator`, `headless`, `tracer`, `audit`, and `dump_assets`. The Android/Ebiten integration is in `mobile/`. Asset extraction and tile utilities live in `tools/`; generated graphics go under `assets/`, and architecture or product images are in `docs/media/`. Keep tests beside the code they cover, named `*_test.go`.

## Build, Test, and Development Commands

- `go test ./...` runs all package tests and verifies the commands compile.
- `make test` is the Makefile entry point for the complete Go test suite.
- `make build` builds the desktop emulator at `bin/emulator`.
- `make run ROM=roms/game.sms` builds and starts the emulator with a ROM.
- `make objects ROM=roms/game.sms FRAMES=6500` removes the prior output for that ROM, then extracts tiles and scenes. Alex Kidd Rev 1 layouts are decoded statically from ROM data; `FRAMES` only controls supplemental sprite sampling.
- `make remaster ROM=...` runs the clean extraction and prepares the AI workspace.
- `make unpack ROM=...`, `make pack ROM=...`, and `make hd ROM=...` prepare and package remastered art for F10.
- `go run ./cmd/headless -frames 120 -out frame.png roms/game.sms` renders frames without the desktop UI.
- `make audit` scans ROMs in `roms/`; `make dump ROM=roms/game.sms` extracts graphics.
- `make release` builds Linux, Windows, and Android artifacts; it requires Docker for Android packaging.

## Coding Style & Naming Conventions

Use `gofmt` for Go files and standard Go naming: exported identifiers use `MixedCaps`, unexported helpers use `mixedCaps`, and tests use `Test<Behavior>`. Keep packages focused on one emulator component or command. Python utilities use four-space indentation and `snake_case` names. No separate lint configuration is currently defined.

## Testing Guidelines

Use Go's standard `testing` package. Add unit tests next to changed code and cover normal behavior plus relevant edge cases; there is no enforced coverage percentage. Run `go test ./...` before submitting changes. Tests should use temporary directories for file output and should not depend on external ROMs, generated assets, or display hardware.

## Commit & Pull Request Guidelines

Recent commits generally use short imperative subjects, often with a type prefix such as `fix:`, `feat:`, or `chore:`. Keep commits focused. Pull requests should explain the behavior change, list validation commands and results, link related issues when applicable, and include screenshots for visible UI or rendering changes.

## Configuration & Generated Files

The emulator reads `gosms_config.json` beside its executable and defaults to `roms/`. Keep local ROMs, generated `assets/`, release outputs, and machine-specific configuration out of commits unless a change explicitly requires them.
