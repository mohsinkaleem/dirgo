.PHONY: build run install clean test bench profile-cpu profile-mem

build:
	go build -o dirgo .

run: build
	./dirgo

install:
	go install .

clean:
	rm -f dirgo cpu.prof mem.prof

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
