build:
	mkdir -p build
	go build -o build/featurevisor-go cli/main.go

test:
	go test ./... -v

clean:
	rm -rf build
