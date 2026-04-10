ZLAW_HOME ?= $(CURDIR)/.zlaw
AGENT     ?= default
BINARY    := $(CURDIR)/zlaw-agent

# Load .env if it exists (secrets stay in make's environment, never in config).
-include .env
export

.PHONY: build serve attach auth-login auth-list test clean help

## build: compile the zlaw-agent binary
build:
	go build -o $(BINARY) ./cmd/zlaw-agent

## serve: build and start the agent daemon
##   make serve
##   make serve AGENT=myagent
##   make serve ZLAW_HOME=/custom/path
serve: build
	ZLAW_HOME=$(ZLAW_HOME) $(BINARY) --agent $(AGENT) serve

## attach: attach a terminal to the running daemon session
##   make attach
##   make attach AGENT=myagent SESSION=mysession
attach: build
	ZLAW_HOME=$(ZLAW_HOME) $(BINARY) --agent $(AGENT) attach $(if $(SESSION),--session $(SESSION),)

## auth-login: set up LLM credentials (interactive)
auth-login: build
	ZLAW_HOME=$(ZLAW_HOME) $(BINARY) auth login

## auth-list: list stored credential profiles
auth-list: build
	ZLAW_HOME=$(ZLAW_HOME) $(BINARY) auth list

## test: run all tests
test:
	go test ./...

## clean: remove the compiled binary
clean:
	rm -f $(BINARY)

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/^## //'
