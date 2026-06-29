.PHONY: build build-no-cache buildx run run-stdin push deploy clean lint test

# Image settings
IMAGE_NAME ?= elasticsearch-mcp
IMAGE_TAG  ?= latest
IMAGE_REG  ?= harbor.<your-domain>.net/infra
IMAGE      ?= $(IMAGE_REG)/$(IMAGE_NAME):$(IMAGE_TAG)

# k8s settings
NAMESPACE  ?= zuoyang
DEPLOYMENT ?= elasticsearch-mcp
CONTAINER  ?= elasticsearch-mcp

# Build args
TARGETOS   ?= linux
TARGETARCH ?= amd64

# ---- Build ----

build:
	docker build \
		--build-arg TARGETOS=$(TARGETOS) \
		--build-arg TARGETARCH=$(TARGETARCH) \
		-t $(IMAGE) \
		.

build-no-cache:
	docker build --no-cache \
		--build-arg TARGETOS=$(TARGETOS) \
		--build-arg TARGETARCH=$(TARGETARCH) \
		-t $(IMAGE) \
		.

buildx:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg TARGETOS=linux \
		--push \
		-t $(IMAGE) \
		.

# ---- Run ----

run:
	docker run --rm \
		-p 8080:8080 \
		-e MCP_TRANSPORT=streamable-http \
		-e MCP_LOG_LEVEL=info \
		-v $(PWD)/etc/elasticsearch.yaml:/app/etc/elasticsearch.yaml:ro \
		$(IMAGE)

run-stdin:
	docker run --rm -it \
		-e MCP_TRANSPORT=stdio \
		-v $(PWD)/etc/elasticsearch.yaml:/app/etc/elasticsearch.yaml:ro \
		$(IMAGE)

# ---- Push ----

push:
	docker push $(IMAGE)

# ---- Deploy (构建 + 推送 + 滚动更新) ----

deploy:
	$(eval TAG := $(shell date +%Y-%m-%d-%H-%M-%S))
	$(eval DEPLOY_IMAGE := $(IMAGE_REG)/$(IMAGE_NAME):$(TAG))
	@echo "=========================================="
	@echo " [1/3] 构建镜像: $(DEPLOY_IMAGE)"
	@echo "=========================================="
	docker build \
		--build-arg TARGETOS=$(TARGETOS) \
		--build-arg TARGETARCH=$(TARGETARCH) \
		-t $(DEPLOY_IMAGE) .
	@echo "=========================================="
	@echo " [2/3] 推送镜像: $(DEPLOY_IMAGE)"
	@echo "=========================================="
	docker push $(DEPLOY_IMAGE)
	@echo "=========================================="
	@echo " [3/3] 更新 deployment/$(DEPLOYMENT) -n $(NAMESPACE)"
	@echo "=========================================="
	kubectl set image "deployment/$(DEPLOYMENT)" "$(CONTAINER)=$(DEPLOY_IMAGE)" -n $(NAMESPACE)
	kubectl rollout status "deployment/$(DEPLOYMENT)" -n $(NAMESPACE) --timeout=120s
	@echo "=========================================="
	@echo " 发版完成: $(DEPLOY_IMAGE)"
	@echo "=========================================="

# ---- Dev ----

lint:
	@which golangci-lint > /dev/null || (echo "Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest")
	@golangci-lint run ./...

test:
	go test -v ./...

clean:
	rm -rf bin/
	docker rmi $(IMAGE) 2>/dev/null || true
