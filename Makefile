.PHONY: build test test-openfeature test-example-1 verify-packages clean setup-monorepo update-monorepo

build:
	mkdir -p build
	go build -o build/featurevisor-go cmd/main.go

test:
	go test ./...
	$(MAKE) test-openfeature

test-openfeature:
	(cd openfeature && GOWORK=off go test ./...)

test-example-1:
	$(MAKE) test
	go run cmd/main.go test --projectDirectoryPath=../featurevisor/examples/example-1 --onlyFailures

verify-packages:
	test "$$(go list -m)" = "github.com/featurevisor/featurevisor-go/v3"
	test "$$(cd openfeature && GOWORK=off go list -m)" = "github.com/featurevisor/featurevisor-go/openfeature/v3"
	(cd openfeature && GOWORK=off go list -deps ./... >/dev/null)

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
