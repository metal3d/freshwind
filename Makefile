
CC:=go
VERSION:=$(shell git describe --tag)
OPT=-X "main.VERSION $(VERSION)"
BUILD:=$(CC) build $(OPT)


all: clean freshwind-linux freshwind-windows freshwind-darwin freshwind-freebsd

freshwind-%: dist 
	 GOOS=$* \
	 $(BUILD) && \
	 mv freshwind.exe freshwind 2>/dev/null || : && \
	 mv freshwind dist/$@

clean:
	rm -rf dist

dist:
	mkdir dist
