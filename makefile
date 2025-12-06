.PHONY: update-env

PROJECT_NAME := tex
PROJECT_ROOT := $(shell pwd)
REPO_PREFIX := ""
VERSION=v0.0.1

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
	@gentool -c ./database/gen.tool

run-dev: 
	@cd ./deploy/dev/ && docker-compose up -d; cd ../../

stop-dev: 
	@cd ./deploy/dev/ && docker-compose stop; cd ../../

clean-dev:
	@cd ./deploy/dev/ && docker-compose down ; cd ../../

ut:
	@echo "work_dir=${PROJECT_ROOT}"
	@mkdir -p ${PROJECT_ROOT}/tmp
	@touch ${PROJECT_ROOT}/tmp/coverage.out
	@chmod +x ${PROJECT_ROOT}/tmp/coverage.out 
	@go test -v -count=1 -gcflags=all=-l -coverprofile=${PROJECT_ROOT}/tmp/coverage.out ./internal/...


build-img:
	docker build -t ${PROJECT_NAME}:${VERSION} . 

push-img:
	time docker push ${PROJECT_NAME}:${VERSION}
