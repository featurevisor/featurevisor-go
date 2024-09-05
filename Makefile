build-executable:
	mkdir -p build
	go build -o build/featurevisor-go cmd/featurevisor-go/main.go

test:
	go test ./types
	go test ./sdk

clean:
	rm -rf build
