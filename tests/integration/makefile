# ==============================================================================
# 🐳 Docker 镜像配置
# ==============================================================================

IMAGE_NAME := oms_server
IMAGE_TAG := latest
DOCKERFILE_SERVER := Dockerfile.server


# ==============================================================================
# 💾 快照配置
# ==============================================================================
SS_DIR := ../../snapshot
# 注意: 容器名称应该与你实际运行的 docker-compose 或 run 命令中的名称保持一致
OMS_CONTAINER := oms_server
ME_CONTAINER := me
CONTAINER_SS_PATH := /app/snapshot
TIMESTAMP := $(shell date +%Y%m%d_%H%M%S)
PAIRS := BTCUSDT ETHUSDT

# ==============================================================================
copy-snapshot:
	@mkdir -p "$(SS_DIR)/$(TIMESTAMP)/server" && mkdir -p "$(SS_DIR)/$(TIMESTAMP)/me"
	@echo "==> Copying Server snapshots from $(OMS_CONTAINER) ..."
	-@docker cp $(OMS_CONTAINER):"$(CONTAINER_SS_PATH)/" "$(SS_DIR)/$(TIMESTAMP)/server/" || echo "No Server snapshots found in $(OMS_CONTAINER)."
	@echo "==> Copying ME snapshots from $(ME_CONTAINER) ..."
# read all pairs
	@for pair in $(PAIRS); do \
		**container_name="$(ME_CONTAINER)_$$pair";** \
		mkdir -p "$(SS_DIR)/$(TIMESTAMP)/me/$$pair/"; \
		docker cp $${container_name}:"$(CONTAINER_SS_PATH)/$$pair/" "$(SS_DIR)/$(TIMESTAMP)/me/$$pair/" || echo "No ME snapshots found for pair $$pair in $${container_name}."; \
	done
	@echo "==> Done. Files copied to $(SS_DIR)/$(TIMESTAMP)"

test:
	@echo "Waiting for services to start..." 
	@chmod +x ../../bin/mvp_client
	@docker-compose down && sleep 2 && docker-compose up -d && sleep 3
	@echo "Running integration tests" && ../../bin/mvp_client ./testcase_massive.case 
	@echo "Wait kafka to be ready..." && sleep 10
	@echo "Dump snapshot" && ../../bin/mvp_client ./testcase_snapshot.case && sleep 1 && $(MAKE) copy-snapshot
# 	@echo "Stopping services..." && docker-compose stop

.PHONY: inter-test copy-snapshot