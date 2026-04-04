APP_NAME    := receipt-system
IMAGE_NAME  := fjlli/receipt-system
PORT        := 8080

.PHONY: run build mod-tidy vet clean docker-build docker-push docker-run docker-pull-run

mod-tidy:
	@if command -v go >/dev/null 2>&1; then \
		go mod tidy; \
	else \
		echo "go mod tidy: スキップ（go が PATH にありません。コミット済みの go.mod / go.sum でビルドします）"; \
	fi

build: mod-tidy
	docker build -t $(IMAGE_NAME) .

run: build
	docker run --rm -p $(PORT):$(PORT) -v $(PWD)/config:/app/config $(IMAGE_NAME)

push: build
	docker push $(IMAGE_NAME)
