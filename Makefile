# tmact build orchestration.
#
# The browser UI is a Vite + React + TypeScript app under internal/web/frontend/.
# Its production build is emitted into internal/web/static/, which the Go server
# embeds via go:embed. Those build artifacts are NOT committed — run `make web`
# (or any target that depends on it) before `go build`. A fresh clone has only
# internal/web/static/.gitkeep, so the binary still compiles; the UI 404s until
# `make web` populates the directory.

SHELL := /usr/bin/env bash
GO ?= go
FRONTEND_DIR := internal/web/frontend
STATIC_DIR := internal/web/static

.PHONY: all build test go-test web-test web web-deps web-dev web-clean run vet fmt clean

all: build

## web-deps: install frontend node modules if missing
web-deps:
	@if [ ! -d "$(FRONTEND_DIR)/node_modules" ]; then \
		echo ">> installing frontend deps"; \
		cd $(FRONTEND_DIR) && npm install; \
	fi

## web: build the React UI into internal/web/static (embedded by go:embed)
web: web-deps
	cd $(FRONTEND_DIR) && npm run build
	@touch $(STATIC_DIR)/.gitkeep

## web-dev: run the Vite dev server (set TMACT_STATUSD to your statusd web-addr)
web-dev: web-deps
	cd $(FRONTEND_DIR) && npm run dev

## web-test: run the frontend unit tests (Vitest)
web-test: web-deps
	cd $(FRONTEND_DIR) && npm test

## web-clean: remove built UI assets but keep the embed placeholder
web-clean:
	find $(STATIC_DIR) -mindepth 1 ! -name .gitkeep -delete
	@touch $(STATIC_DIR)/.gitkeep

## build: build the frontend, then the Go binaries
build: web
	$(GO) build ./...

## go-test: run only the Go test suite (assumes the UI is already built)
go-test:
	$(GO) test ./...

## test: build the frontend, then run frontend + Go tests
test: web web-test
	$(GO) test ./...

## run: build the frontend, then run statusd (pass ARGS="statusd start --web-addr :8080")
run: web
	$(GO) run ./cmd/tmact $(ARGS)

vet: web
	$(GO) vet ./...

clean: web-clean
