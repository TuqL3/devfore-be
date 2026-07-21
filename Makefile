.DEFAULT_GOAL := help
SHELL := /bin/bash

-include .env
export

MIGRATE := docker run --rm --network=host -v $(PWD)/migrations:/migrations migrate/migrate:v4.18.1

.PHONY: help
help: ## Hiện danh sách lệnh
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: up
up: ## Chạy backend stack (postgres + api)
	docker compose up -d --build

.PHONY: down
down: ## Dừng stack
	docker compose down

.PHONY: logs
logs: ## Xem log
	docker compose logs -f

.PHONY: migrate
migrate: ## Chạy migration lên bản mới nhất
	$(MIGRATE) -path=/migrations -database "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Rollback 1 bước
	$(MIGRATE) -path=/migrations -database "$(DATABASE_URL)" down 1

.PHONY: migrate-new
migrate-new: ## Tạo migration mới: make migrate-new name=add_courses
	@test -n "$(name)" || (echo "cần: make migrate-new name=<ten>"; exit 1)
	$(MIGRATE) create -ext sql -dir /migrations -seq $(name)

.PHONY: lint
lint: ## Lint Go
	go vet ./... && gofmt -l . | (! grep .)

.PHONY: test
test: ## Chạy test
	go test ./...

.PHONY: build
build: ## Build (kiểm tra compile)
	go build ./...
