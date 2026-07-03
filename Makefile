.PHONY: build vet fmt test check

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
	wails dev -tags "$(GUI_TAGS)"

gui-build:
	wails build -tags "$(GUI_TAGS)"
