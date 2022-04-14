# ==============================================================================
# Build & Test

build:
	go build ./...

install:
	go install ./...

image:
	docker build -t gocoin:latest .


local:
	docker-compose build
	docker-compose run gc-local

test:
	go test -v -p=1 -timeout=0 ./...

# ==============================================================================
# Modules support
tidy:
	go mod tidy
	go mod vendor