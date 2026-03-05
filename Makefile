.PHONY: all crflc crfls

EXEEXT:=

ifeq ($(OS),Windows_NT)
	EXEEXT := .exe
endif

all: crflc crfls

crflc:
	go build -o bin/crflc$(EXEEXT) crfl/src/crflc/

crfls:
	go build -o bin/crfls$(EXEEXT) crfl/src/crfls/
