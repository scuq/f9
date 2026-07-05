.PHONY: build vet fmt test check bump

VERSION := $(shell cat VERSION 2>/dev/null || echo dev)
LDFLAGS := -X github.com/scuq/f9/internal/app.Version=$(VERSION)

build:
	go build ./...

vet:
	go vet ./...

fmt:
	@out=$$(gofmt -l cmd internal); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

test:
	go test ./...

check: build vet fmt test

# cross-compile smoke for the full target matrix (phase 00 exit criterion)
matrix:
	@for os in linux darwin windows; do for arch in amd64 arm64; do \
		echo "== $$os/$$arch"; GOOS=$$os GOARCH=$$arch go build ./... || exit 1; \
	done; done

# GUI (Wails). Debian 13 ships webkit2gtk 4.1 only, hence the extra tag.
GUI_TAGS := gui,webkit2_41

gui-dev:
	WEBKIT_DISABLE_DMABUF_RENDERER=1 wails dev -tags "$(GUI_TAGS)" -ldflags "$(LDFLAGS)"

gui-build:
	WEBKIT_DISABLE_DMABUF_RENDERER=1 wails build -tags "$(GUI_TAGS)" -ldflags "$(LDFLAGS)"

# bump the version, commit, and tag:  make bump V=1.2.3
bump:
	@bash scripts/bump.sh $(V)
