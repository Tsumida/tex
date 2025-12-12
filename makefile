.PHONY: update-env

PROJECT_NAME := tex
PROJECT_ROOT := $(shell pwd)
REPO_PREFIX := ""

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

ut:
	@echo "work_dir=${PROJECT_ROOT}"
	@mkdir -p ${PROJECT_ROOT}/tmp
	@touch ${PROJECT_ROOT}/tmp/coverage.out
	@chmod +x ${PROJECT_ROOT}/tmp/coverage.out 
	@go test -v -count=1 -gcflags=all=-l -coverprofile=${PROJECT_ROOT}/tmp/coverage.out ./pkg/...

it:
	@echo "work_dir=${PROJECT_ROOT}"
	@mkdir -p ${PROJECT_ROOT}/tmp
	@touch ${PROJECT_ROOT}/tmp/it_coverage.out
	@chmod +x ${PROJECT_ROOT}/tmp/it_coverage.out 
	@go test -v -count=1 -gcflags=all=-l -coverprofile=${PROJECT_ROOT}/tmp/it_coverage.out ./tests/tex/...

# ==============================================================================
# 💾 构建指令
# ==============================================================================
build-img:
	docker build -t ${PROJECT_NAME}:latest . 

push-img:
	time docker push ${PROJECT_NAME}:latest