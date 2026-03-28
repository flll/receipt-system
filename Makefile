APP_NAME    := receipt-system
IMAGE_NAME  := fjlli/receipt-system
PORT        := 8080

.PHONY: run build vet clean docker-build docker-push docker-run docker-pull-run

run: build
	docker run --rm -p $(PORT):$(PORT) -v $(PWD)/config:/app/config $(IMAGE_NAME)

build: 
	docker build -t $(IMAGE_NAME) .

push: build
	docker push $(IMAGE_NAME)

