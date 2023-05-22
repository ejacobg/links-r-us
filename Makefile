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

# proto-deps: install protobuf and gRPC packages
proto-deps:
	@echo "[go get] ensuring protoc packages are available"
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@go get google.golang.org/protobuf
	@go get google.golang.org/grpc