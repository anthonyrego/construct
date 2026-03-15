.PHONY: build run clean vet fmt

build:
	go build -o build/construct .

run: build
	./build/construct

clean:
	rm -rf build/

vet:
	go vet ./...

fmt:
	gofmt -w .
