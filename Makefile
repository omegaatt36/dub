.PHONY: generate dev test test-coverage build clean frontend-prep

generate:
	templ generate

frontend-prep:
	mkdir -p frontend/dist
	cp web/index.html frontend/dist/
	cp -r web/static frontend/dist/

dev: generate frontend-prep
	templ generate -watch & wails dev

test:
	go test -v ./internal/... ./app/...

test-coverage:
	go test -cover ./internal/... ./app/...

build: generate frontend-prep
	wails build

clean:
	rm -rf build/ frontend/dist/
