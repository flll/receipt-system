APP_NAME    := receipt-system
IMAGE_NAME  := fjlli/receipt-system
PORT        := 8080

.PHONY: run build vet clean docker-build docker-push docker-run docker-pull-run

# ✷ ビルドしてプッシュ
push: build
	docker push $(IMAGE_NAME)

run: build
	docker run --rm -p $(PORT):$(PORT) -v $(PWD)/config:/app/config $(IMAGE_NAME)

# ✷ Docker 内で静的解析
vet:
	docker run --rm golang:1.25 go vet ./...

# ✷ ビルドして実行
build: 
	docker build -t $(IMAGE_NAME) .

# ✷ Docker Hub のイメージを pull して実行
docker-pull-run:
	docker run --rm -p $(PORT):$(PORT) -v $(PWD)/config:/app/config $(IMAGE_NAME)
