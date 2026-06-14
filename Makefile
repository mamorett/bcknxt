# bcknxt Makefile
# Build targets for OSX arm64, Linux arm64, Linux amd64

GO       ?= go
OUTDIR   ?= bin
MODULE   := bcknxt

.PHONY: all build clean run \
        build-osx-arm64 build-linux-arm64 build-linux-amd64 build-all

all: build

build:
	@echo "  Building for current platform..."
	$(GO) build -o $(OUTDIR)/$(MODULE) .
	@echo "  -> $(OUTDIR)/$(MODULE)"

build-osx-arm64:
	@echo "  Building for OSX arm64..."
	GOOS=darwin GOARCH=arm64 $(GO) build -o $(OUTDIR)/bcknxt-darwin-arm64 .
	@echo "  -> $(OUTDIR)/bcknxt-darwin-arm64"

build-linux-arm64:
	@echo "  Building for Linux arm64..."
	GOOS=linux GOARCH=arm64 $(GO) build -o $(OUTDIR)/bcknxt-linux-arm64 .
	@echo "  -> $(OUTDIR)/bcknxt-linux-arm64"

build-linux-amd64:
	@echo "  Building for Linux amd64..."
	GOOS=linux GOARCH=amd64 $(GO) build -o $(OUTDIR)/bcknxt-linux-amd64 .
	@echo "  -> $(OUTDIR)/bcknxt-linux-amd64"

build-all: build-osx-arm64 build-linux-arm64 build-linux-amd64
	@echo "  All builds complete."

run:
	$(GO) run . $(filter-out $@,$(MAKECMDGOALS))

clean:
	@echo "  Cleaning..."
	rm -rf $(OUTDIR)
	@echo "  Done."

# Allow passing arguments to `make run`
%:
	@:
