.PHONY: build run dev test clean

build:
	go build -o queue-discover

run: build
	./queue-discover

dev:
	go run main.go

test:
	go test ./...

clean:
	rm -f queue-discover