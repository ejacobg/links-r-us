# Include variables from the .envrc file.
include .envrc

# ==================================================================================== #
# HELPERS
# ==================================================================================== #

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

.PHONY: confirm
confirm:
	@echo -n 'Are you sure? [y/N] ' && read ans && [ $${ans:-N} = y ]

define dsn_missing_error

CDB_DSN env var is undefined. To run the migrations this envvar
must point to a cockroach db instance. For example, if you are
running a local cockroachdb (with --insecure) and have created
a database called 'linkgraph' you can define the envvar by
running:

export CDB_DSN='postgresql://root@localhost:26257/linkgraph?sslmode=disable'

endef
export dsn_missing_error

check-cdb-env:
ifndef CDB_DSN
	$(error ${dsn_missing_error})
endif

# ==================================================================================== #
# DEPENDENCIES
# ==================================================================================== #

# install/gomock: download the GoMock framework and associated mockgen tool
.PHONY: install/gomock
install/gomock:
	go get -u github.com/golang/mock/gomock
	go install github.com/golang/mock/mockgen@latest

# install/proto: download protoc packages (download the protoc compiler on your own)
.PHONY: install/proto
install/proto:
	@echo "[go get] ensuring protoc packages are available"
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go get google.golang.org/protobuf
	go get google.golang.org/grpc

# ==================================================================================== #
# DEVELOPMENT
# ==================================================================================== #

# test: run all tests
.PHONY: test
test:
	@echo 'Running tests...'
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

# cdb/migrations/up: apply all CockroachDB migrations
.PHONY: cdb/migrations/up
cdb/migrations/up: confirm check-cdb-env
	@echo 'Running up migrations...'
	migrate -path ./cdb/migrations -database ${CDB_DSN} up

# cdb/start: start a local CockroachDB instance. Stop with Ctrl-C.
.PHONY: cdb/start
cdb/start:
	cockroach start-single-node --insecure --advertise-addr 127.0.0.1:26257

# cdb/setup: create the linkgraph database; should only need to be run once
.PHONY: cdb/setup
cdb/setup:
	cockroach sql --insecure -e "CREATE DATABASE linkgraph;"

# es/start: start a local Elasticsearch instance. Stop with Ctrl-C.
# Note: if using Linux, use bin/elasticsearch (not .bat)
# Note 2: requires disabled security. Go to ${ES_HOME}/config/elasticsearch.yml and add this line:
#	xpack.security.enabled: false
.PHONY: es/start
es/start:
	${ES_HOME}/bin/elasticsearch.bat

# ==================================================================================== #
# BUILD
# ==================================================================================== #

SHELL=/bin/bash -o pipefail
IMAGE = linksrus-monolith
SHA = $(shell git rev-parse --short HEAD)

ifeq ($(origin PRIVATE_REGISTRY),undefined)
	PRIVATE_REGISTRY := $(shell minikube ip 2>/dev/null):5000
endif

ifneq ($(PRIVATE_REGISTRY),)
	PREFIX:=${PRIVATE_REGISTRY}/
endif

.PHONY: dockerize
dockerize:
	@echo "[docker build] building ${IMAGE} (tags: ${PREFIX}${IMAGE}:latest, ${PREFIX}${IMAGE}:${SHA})"
	@docker build --file ./Dockerfile \
		--tag ${PREFIX}${IMAGE}:latest \
		--tag ${PREFIX}${IMAGE}:${SHA} \
		. 2>&1 | sed -e "s/^/ | /g"

.PHONY: push
push:
	@echo "[docker push] pushing ${PREFIX}${IMAGE}:latest"
	@docker push ${PREFIX}${IMAGE}:latest 2>&1 | sed -e "s/^/ | /g"
	@echo "[docker push] pushing ${PREFIX}${IMAGE}:${SHA}"
	@docker push ${PREFIX}${IMAGE}:${SHA} 2>&1 | sed -e "s/^/ | /g"

.PHONY: dockerize-and-push
dockerize-and-push: dockerize push
