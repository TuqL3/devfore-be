.DEFAULT_GOAL := help
SHELL := /bin/bash

-include .env
export

MIGRATE := docker run --rm --network=host -v $(PWD)/migrations:/migrations migrate/migrate:v4.18.1

.PHONY: help
help: ## Hiện danh sách lệnh
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: up
up: ## Bật postgres (docker)
	docker compose up -d

.PHONY: down
down: ## Tắt postgres
	docker compose down

.PHONY: migrate
migrate: ## Dựng tất cả: postgres, migration, dữ liệu demo, image lab
	docker compose up -d
	@printf 'chờ postgres'; \
	 for i in $$(seq 1 60); do \
	   docker compose exec -T postgres pg_isready -U "$(DB_USER)" -q && break; \
	   printf '.'; sleep 1; \
	 done; \
	 echo ' sẵn sàng'
	$(MIGRATE) -path=/migrations -database "$(DATABASE_URL)" up
	@$(MAKE) --no-print-directory seed
	@$(MAKE) --no-print-directory lab-images

# Chỉ schema, không dữ liệu demo và không image — dùng khi chạy ở môi trường
# không phải máy local, nơi `seed` sẽ chèn tài khoản demo vào nơi không nên có.
.PHONY: migrate-schema
migrate-schema: ## Chỉ chạy migration (không seed, không build image)
	$(MIGRATE) -path=/migrations -database "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Lùi 1 migration gần nhất
	$(MIGRATE) -path=/migrations -database "$(DATABASE_URL)" down 1

.PHONY: migrate-new
migrate-new: ## Tạo migration: make migrate-new name=add_courses
	@test -n "$(name)" || (echo "cần: make migrate-new name=<ten>"; exit 1)
	$(MIGRATE) create -ext sql -dir /migrations -seq $(name)

.PHONY: seed
seed: ## Nạp dữ liệu demo (đã nằm trong `make migrate`)
	docker compose exec -T postgres psql -U "$(DB_USER)" -d "$(DB_NAME)" < scripts/seed.sql

.PHONY: admin
admin: ## Tạo/cấp quyền admin: make admin email=a@b.c password=... [username=...]
	@test -n "$(email)" -a -n "$(password)" || (echo "cần: make admin email=<email> password=<mật khẩu>"; exit 1)
	go run ./cmd/createadmin -email "$(email)" -password "$(password)" $(if $(username),-username "$(username)")

.PHONY: lab-images
lab-images: ## Build 4 image lab (đã nằm trong `make migrate`)
	@for img in linux git docker net; do \
		echo "==> devforge/$$img:latest"; \
		docker build -q -f labs/$$img/Dockerfile -t devforge/$$img:latest labs || exit 1; \
	done

.PHONY: check-seed
check-seed: ## Chạy mọi check_script trong seed thật sự trong container lab
	bash scripts/check-seed.sh

.PHONY: air
air: ## Chạy api hot reload (host, cần `make up` trước)
	air

.PHONY: test
test: ## Chạy test
	go test ./...
