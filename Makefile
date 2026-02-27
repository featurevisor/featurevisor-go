.PHONY: build test clean setup-monorepo update-monorepo

build:
	mkdir -p build
	go build -o build/featurevisor-go cmd/main.go

test:
	go test ./... -v

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
