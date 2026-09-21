.DEFAULT_GOAL := help
SHELL := /bin/bash

-include .env
export

# Development brings up the dev-profile services (mailpit) explicitly rather
# than leaning on COMPOSE_PROFILES in .env: an .env written before the profile
# existed would silently lose the mail catcher. Production composes with -f and
# never passes this, which is what keeps mailpit off a public box.
DC := docker compose --profile dev

MIGRATE := docker run --rm --network=host -v $(PWD)/migrations:/migrations migrate/migrate:v4.18.1

.PHONY: help
help: ## Hiện danh sách lệnh
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: up
up: ## Bật postgres (docker)
	$(DC) up -d

.PHONY: down
down: ## Tắt postgres
	$(DC) down

.PHONY: migrate
migrate: ## Dựng tất cả: postgres, migration, dữ liệu demo, image lab
	$(DC) up -d
	@printf 'chờ postgres'; \
	 for i in $$(seq 1 60); do \
	   $(DC) exec -T postgres pg_isready -U "$(DB_USER)" -q && break; \
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
	$(DC) exec -T postgres psql -U "$(DB_USER)" -d "$(DB_NAME)" < scripts/seed.sql

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

# Không cần docker: bài mô phỏng không có container nào để chạy vào. Cần
# database vì kịch bản và điều kiện chấm là dữ liệu, không phải code.
.PHONY: check-sim
check-sim: ## Chấm thử mọi nhiệm vụ mô phỏng: pipeline sai phải trượt, pipeline đúng phải đậu
	go run ./cmd/checksim

.PHONY: check-edge
check-edge: ## Kiểm route của deploy/nginx/devforge.conf (cần docker; chỉ prod mới có edge)
	./scripts/edge-routes.check.sh

.PHONY: check-release-guard
check-release-guard: ## Kiểm scripts/release-guard.sh — tên tag nào được phép thành bản phát hành
	./scripts/release-guard.check.sh

.PHONY: air
air: ## Chạy api hot reload (host, cần `make up` trước)
	air

.PHONY: test
test: ## Chạy test
	go test ./...

# The frontend clone. The directory name differs between the box (devforge-fe,
# as README §11 and the deploy paths spell it) and some working copies cloned
# under the repo slug, so take whichever is actually there rather than making
# every developer edit this line.
FE ?= $(firstword $(wildcard ../devforge-fe ../devfore-fe))

# Same shape as `make test`, and the same precondition: this Makefile exports
# .env, so the tests that skip themselves without a database will instead try to
# reach one. Run `make up` first, or read the number CI prints.
.PHONY: cover
cover: ## Đo coverage như CI đo, sàn 16.5% (cần `make up` trước, như `make test`)
	go test -coverprofile=cover.out ./...
	@go tool cover -func=cover.out | tail -1

# A release is one version number worn by both repos. Nothing enforces that in
# git — two repositories have two independent tag namespaces and only the name
# ties them together — so the forgettable half is scripted here. The gate that
# catches it anyway lives in CI: devforge-be's deploy job waits for the frontend
# image at the same tag and fails the release if it never appears.
.PHONY: release
release: ## Gắn tag cả hai repo rồi push: make release v=v1.2.0
	@test -n "$(v)" || { echo "usage: make release v=v1.2.0"; exit 1; }
	@test -n "$(FE)" || { echo "no frontend clone next to this one"; exit 1; }
	@# Same guard CI runs, so a name that is refused here is refused there and
	@# a name accepted here cannot surprise the deploy job.
	@./scripts/release-guard.sh "$(v)"
	@# Never move a tag that already exists. The image for it has been built and
	@# possibly deployed; repointing the name makes that release unreproducible
	@# and a rollback a guess. Cut v1.2.1 instead.
	@for d in . $(FE); do \
	  git -C $$d rev-parse -q --verify refs/tags/$(v) >/dev/null \
	    && { echo "$$d already has $(v) — cut the next patch version instead"; exit 1; }; \
	  test -z "$$(git -C $$d status --porcelain)" \
	    || { echo "$$d has uncommitted changes"; exit 1; }; \
	done; true
	@for d in . $(FE); do \
	  git -C $$d tag -a $(v) -m $(v) && git -C $$d push origin $(v) || exit 1; \
	done
	@echo "==> tagged $(v) in both repos; watch Actions for the promote and deploy"
