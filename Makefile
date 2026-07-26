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
migrate: ## Chạy migration
	$(MIGRATE) -path=/migrations -database "$(DATABASE_URL)" up

.PHONY: migrate-new
migrate-new: ## Tạo migration: make migrate-new name=add_courses
	@test -n "$(name)" || (echo "cần: make migrate-new name=<ten>"; exit 1)
	$(MIGRATE) create -ext sql -dir /migrations -seq $(name)

.PHONY: seed
seed: ## Nạp dữ liệu demo (local, cần `make up` + `make migrate` trước)
	docker compose exec -T postgres psql -U "$(DB_USER)" -d "$(DB_NAME)" < scripts/seed.sql

.PHONY: air
air: ## Chạy api hot reload (host, cần `make up` trước)
	air

.PHONY: test
test: ## Chạy test
	go test ./...
