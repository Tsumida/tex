.PHONY: update-env

PROJECT_NAME := tex
PROJECT_ROOT := $(shell pwd)
REPO_PREFIX := ""


# ==============================================================================
# 🐳 Docker 镜像配置
# ==============================================================================

IMAGE_NAME := oms_server
IMAGE_TAG := latest
DOCKERFILE_SERVER := Dockerfile.server


# ==============================================================================
# 💾 快照配置
# ==============================================================================
SS_DIR := ./tmp/snapshot
# 注意: 容器名称应该与你实际运行的 docker-compose 或 run 命令中的名称保持一致
OMS_CONTAINER := oms_server
ME_CONTAINER := me
CONTAINER_SS_PATH := /app/snapshot
TIMESTAMP := $(shell date +%Y%m%d_%H%M%S)
PAIRS := BTCUSDT ETHUSDT
# ==============================================================================
# 💾 集成环境
# ==============================================================================
DOCKER_COMPOSE_FILE := ./tests/integration/docker-compose.yaml

# ==============================================================================
# 💾 本地开发
# ==============================================================================
list-env:
	@find ./ -type f -name "*.go" \
	-exec grep -oP 'os.Getenv\(\K"[A-Z_]+"(?=\))' {} \; \
	| sort \
	| uniq \
	| awk '{gsub(/"/, "", $$0); print "ENV", $$0 "=\"\""}'

list-redis-key:
	@find ./ -type f -name "*.go" \
	-exec grep -oP 'REDIS_KEY_[A-Z0-9_]+\s*=\s*"[a-z_\-]+"' {} \; \
	| awk -F'"' '{gsub(/REDIS_KEY_/, "", $$1); print $$2}'

pb:
	@buf generate -v --path ./api

model:
	gentool -dsn "tex:n8btp3yfl0arxb12@tcp(localhost:3306)/tex?charset=utf8mb4&parseTime=true" \
        -db mysql \
        -outPath ./pkg/model \
        -modelPkgName model \
        -onlyModel true \
        -fieldSignable true \
		-withUnitTest true

# ==============================================================================
# 💾 测试
# ==============================================================================
ut:
	@echo "work_dir=${PROJECT_ROOT}"
	@mkdir -p ${PROJECT_ROOT}/tmp
	@touch ${PROJECT_ROOT}/tmp/coverage.out
	@chmod +x ${PROJECT_ROOT}/tmp/coverage.out 
	@go test -v -count=1 -gcflags=all=-l -coverprofile=${PROJECT_ROOT}/tmp/coverage.out ./pkg/...
	@go tool cover -func=${PROJECT_ROOT}/tmp/coverage.out | grep total | awk '{print "Total Coverage: " $$3}'

copy-snapshot:
	@set -e; \
	TIMESTAMP=$$(date +%Y%m%d_%H%M%S); \
	OUT_DIR=$(SS_DIR); \
	\
	echo "==> Copying OMS snapshots from oms_server ..."; \
	if docker ps --format '{{.Names}}' | grep -q '^oms_server$$'; then \
		docker cp oms_server:/app/snapshot/. $$OUT_DIR/; \
	else \
		echo "OMS container oms_server not running"; \
	fi; \
	\
	for pair in BTCUSDT ETHUSDT; do \
		container=me_$$pair; \
		echo "==> Copying ME snapshots from $$container ..."; \
		if docker ps --format '{{.Names}}' | grep -q "^$$container$$"; then \
			docker cp $$container:/app/snapshot/. $$OUT_DIR/; \
		else \
			echo "Container $$container not running"; \
		fi; \
	done; \
	\
	echo "==> Done. Files copied to $$OUT_DIR"

test:
	@echo "Cleaning up previous test data..." && rm -rf $(SS_DIR) && mkdir -p $(SS_DIR)
	@echo "Waiting for services to start..." 
	@chmod +x ./bin/mvp_client
	@docker-compose -f $(DOCKER_COMPOSE_FILE) down  && sleep 2 && docker-compose -f $(DOCKER_COMPOSE_FILE) up -d && sleep 3
	@echo "Running integration tests" && ./bin/mvp_client ./tests/integration/testcase_massive.case 
	@echo "Wait kafka to be consumed..." && sleep 10
	@echo "Dump snapshot" && ./bin/mvp_client ./tests/integration/testcase_snapshot.case && sleep 2 && $(MAKE) copy-snapshot
	@echo "Checking snapshot consistency..." && python3 tests/data/snapshot_check.py --dir=$(SS_DIR)
# 	@echo "Stopping services..." && docker-compose stop

# ==============================================================================
# 💾 构建指令
# ==============================================================================
build-img:
	docker build -t ${PROJECT_NAME}:latest . 

push-img:
	time docker push ${PROJECT_NAME}:latest