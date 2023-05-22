# See https://grpc.io/docs/languages/go/quickstart/#regenerate-grpc-code
.PHONY: proto
proto:
	@echo "[protoc] generating protos for API"
	@protoc --go_out=proto/ --go_opt=paths=source_relative \
         --go-grpc_out=proto/ --go-grpc_opt=paths=source_relative \
         -Iproto/ api.proto
