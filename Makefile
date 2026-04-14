ZLAW_HOME ?= $(CURDIR)/.zlaw
AGENT     ?= default
BINARY    := $(CURDIR)/zlaw

# Load .env if it exists (secrets stay in make's environment, never in config).
-include .env
export

.PHONY: build serve attach auth-login auth-list test clean hooks help

## build: compile the zlaw binary
build:
	go build -o $(BINARY) ./cmd/zlaw

## serve: build and start the agent daemon
##   make serve
##   make serve AGENT=myagent
##   make serve ZLAW_HOME=/custom/path
serve: build
	ZLAW_HOME=$(ZLAW_HOME) $(BINARY) agent serve --agent $(AGENT)

## attach: attach a terminal to the running daemon session
##   make attach
##   make attach AGENT=myagent SESSION=mysession
attach: build
	ZLAW_HOME=$(ZLAW_HOME) $(BINARY) agent attach --agent $(AGENT) $(if $(SESSION),--session $(SESSION),)

## auth-login: set up LLM credentials (interactive)
auth-login: build
	ZLAW_HOME=$(ZLAW_HOME) $(BINARY) auth login

## auth-list: list stored credential profiles
auth-list: build
	ZLAW_HOME=$(ZLAW_HOME) $(BINARY) auth list

## hooks: install git hooks from .githooks/
hooks:
	@for hook in .githooks/*; do \
		name=$$(basename $$hook); \
		cp $$hook .git/hooks/$$name; \
		chmod +x .git/hooks/$$name; \
		echo "installed .git/hooks/$$name"; \
	done

## test: run all tests
test:
	go test ./...

## clean: remove the compiled binary
clean:
	rm -f $(BINARY)

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/^## //'
