build-executable:
	mkdir -p build
	go build -o build/featurevisor-go cli/main.go

test:
	go test ./sdk -v

clean:
	rm -rf build
