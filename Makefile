.PHONY: build run install clean test bench profile-cpu profile-mem release

VERSION ?= dev
LDFLAGS = -ldflags="-s -w -X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o dirgo .

run: build
	./dirgo

install:
	go install $(LDFLAGS) .

clean:
	rm -f dirgo cpu.prof mem.prof
	rm -rf dist

test:
	go test -v ./...

bench:
	go test -bench=. -benchmem ./...

profile-cpu:
	go test -cpuprofile=cpu.prof -bench=BenchmarkScanDirectory ./...
	go tool pprof -http=:8080 cpu.prof

profile-mem:
	go test -memprofile=mem.prof -bench=BenchmarkScanDirectory ./...
	go tool pprof -http=:8080 mem.prof

release:
	@mkdir -p dist
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o dist/dirgo-darwin-arm64 .
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o dist/dirgo-darwin-amd64 .
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o dist/dirgo-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o dist/dirgo-linux-arm64 .
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/dirgo-windows-amd64.exe .
