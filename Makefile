APP_NAME    := receipt-system
IMAGE_NAME  := fjlli/receipt-system
PORT        := 8080

.PHONY: run build mod-tidy vet clean docker-build docker-push docker-run docker-pull-run

mod-tidy:
	go mod tidy

build: mod-tidy
	docker build -t $(IMAGE_NAME) .

run: build
	docker run --rm -p $(PORT):$(PORT) -v $(PWD)/config:/app/config $(IMAGE_NAME)

push: build
	docker push $(IMAGE_NAME)
