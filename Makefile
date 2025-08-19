# Include variables from the .envrc file
include .envrc

# ==================================================================================== #
# HELPERS
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo Usage:
	@sed -n s/^##//p ${MAKEFILE_LIST} | column -t -s : |  sed -e s/^/ /

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

## run/api: run the cmd/api application 
.PHONY: run/api
run/api:
	@go run ./cmd/api

## run/air: default build the cmd/api application 
.PHONY: run/build 
run/build:
	@echo Building cmd/api...
	go build -ldflags='-s' -o=./bin/api ./cmd/api

## run/air: build the cmd/api AIR application with autoreload
.PHONY: run/air 
run/air:
	@echo Building AIR application with autoreload...
	@air -c .air.toml 

## db/psql: connect to the database using psql
.PHONY: db/psql
db/psql:
	@psql cinemesis

## db/migrations/new name=$1: create a new database migration
.PHONY: 
db/migrations/new:
	@echo Creating migration files for ${name}...
	migrate create -seq -ext=.sql -dir=./migrations ${name}

## db/migrations/up: apply all up database migrations
.PHONY: db/migrations/new
db/migrations/up:
	@echo Running migrations...
	migrate -path ./migrations -database ${POSTGRESQL_CONN} up

## swag : create swagger docs
.PHONY: swag
swag:
	@echo Create Swagger docs...
	swag init -g cmd/api/main.go

## swag/clean : clean swagger docs
.PHONY: swag/clean
swag/clean:
	@echo Cleaning old Swagger docs...
	@rm -rf docs

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## tidy: tidy module dependencies and format all .go files
.PHONY: tidy
tidy:
	@echo Tidying module dependencies...
	go mod tidy
	@echo Verifying and vendoring module dependencies...
	go mod verify
	go mod vendor
	@echo Formatting .go files...
	go fmt ./...

## audit: run quality control checks
.PHONY: audit
audit:
	@echo Checking module dependencies...
	go mod tidy -diff
	go mod verify
	@echo Vetting code...
	go vet ./...
	go tool staticcheck ./...
	@echo Running tests...
	go test -race -vet=off ./...


# Existing Makefile content...

# ==================================================================================== #
# DOCKER
# ==================================================================================== #

## rebuild: rebuild the Docker containers without cache
.PHONY: docker/rebuild
docker/rebuild:
	docker-compose down
	docker-compose build --no-cache
	docker-compose up -d

## docker/build: build the Docker image
.PHONY: docker/build
docker/build:
	@echo 'Building Docker image...'
	docker-compose build

## docker/restart-api: restart the API container
.PHONY: docker/restart-api
docker/restart-api:
	docker-compose restart api

## docker/up: start the Docker containers
.PHONY: docker/up
docker/up:
	@echo 'Starting Docker containers...'
	docker-compose up -d

## docker/down: stop the Docker containers
.PHONY: docker/down
docker/down:
	@echo 'Stopping Docker containers...'
	docker-compose down

## docker/logs: view Docker logs
.PHONY: docker/logs
docker/logs:
	@echo 'Viewing Docker logs...'
	docker-compose logs -f

## docker/db/migrations/up: apply migrations in Docker
.PHONY: docker/db/migrations/up
docker/db/migrations/up:
	@echo 'Running migrations in Docker...'
	docker-compose exec api migrate -path /app/migrations -database ${DOCKER_POSTGRESQL_CONN} up

## docker/db/migrations/down: apply migrations in Docker
.PHONY: docker/db/migrations/down
docker/db/migrations/down:
	@echo 'Running migrations down in Docker...'
	docker-compose exec api migrate -path /app/migrations -database ${DOCKER_POSTGRESQL_CONN} down 1