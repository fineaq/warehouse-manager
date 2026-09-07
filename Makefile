.PHONY: db-up db-down db-nuke migrate-up migrate-down migrate-new db-reset db-shell

include .env
export

DB_URL  = postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@postgres:5432/$(POSTGRES_DB)?sslmode=disable
MIGRATE = docker compose run --rm migrate -path /migrations -database "$(DB_URL)"

db-up:
	docker compose up -d --wait postgres

db-down:
	docker compose down

db-nuke:
	docker compose down -v

migrate-up: db-up
	$(MIGRATE) up

migrate-down: db-up
	$(MIGRATE) down 1

migrate-new:
	docker compose run --rm migrate create -ext sql -dir /migrations -seq $(name)

db-reset:
	$(MAKE) db-nuke
	$(MAKE) db-up
	$(MAKE) migrate-up

db-shell:
	docker compose exec postgres psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)
