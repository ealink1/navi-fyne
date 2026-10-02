.PHONY: build run package drivers test vet selfcheck
build:
	python3 tools/build.py
run: build
	./bin/navi-fyne
package:
	python3 tools/build.py --package
drivers:
	python3 tools/build.py --all-drivers --skip-app
test:
	go test ./...
vet:
	go vet ./...
selfcheck:
	python3 tools/check-architecture.py
	go run ./tools/check-go-size
	python3 tools/verify-upstream.py
	python3 tools/verify-ui-assets.py
	go test ./...
	go test -tags gonavi_full_drivers ./internal/upstream/db ./cmd/driver-agent
	go test -race ./internal/application ./internal/infra/... ./internal/domain ./internal/ui ./cmd/release-sign
	go vet ./...
	python3 tools/build.py --all-drivers --package
