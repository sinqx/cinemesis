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

.PHONY: confirm
confirm:
	@echo -n Are you sure? [y/N]  && read ans && [ $${ans:-N} = y ]


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
