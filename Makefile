.PHONY: build test test-example-1 clean setup-monorepo update-monorepo

build:
	mkdir -p build
	go build -o build/featurevisor-go cmd/main.go

test:
	go test ./...

test-example-1:
	go test ./...
	go run cmd/main.go test --projectDirectoryPath=../featurevisor/examples/example-1 --onlyFailures

clean:
	rm -rf build

setup-monorepo:
	mkdir -p monorepo
	if [ ! -d "monorepo/.git" ]; then \
		git clone git@github.com:featurevisor/featurevisor.git monorepo; \
	else \
		(cd monorepo && git fetch origin main && git checkout main && git pull origin main); \
	fi
	(cd monorepo && make install && make build)

update-monorepo:
	(cd monorepo && git pull origin main)
