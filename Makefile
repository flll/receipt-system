APP_NAME    := receipt-system
IMAGE_NAME  := fjlli/receipt-system
PORT        := 8080

.PHONY: run build clean docker-build docker-run docker-pull-run vet

# ✷ ローカル実行
run:
	go run .

# ✷ バイナリビルド
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(APP_NAME) .

# ✷ 静的解析
vet:
	go vet ./...

# ✷ クリーンアップ
clean:
	rm -f $(APP_NAME)

# ✷ Docker イメージビルド
docker-build:
	docker build -t $(IMAGE_NAME) .

# ✷ ローカルビルドしたイメージを実行
docker-run: docker-build
	docker run --rm -p $(PORT):$(PORT) -v $(PWD)/config:/app/config $(IMAGE_NAME)

# ✷ Docker Hub のイメージを pull して実行
docker-pull-run:
	docker run --rm -p $(PORT):$(PORT) -v $(PWD)/config:/app/config $(IMAGE_NAME)
