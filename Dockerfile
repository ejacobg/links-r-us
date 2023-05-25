# Denote the base container we will start with, and alias it using AS.
FROM golang:1.13 AS builder

# Create a new directory to hold our source code.
WORKDIR $GOPATH/src/github.com/ejacobg/links-r-us

# Copy the contents of our project root into the directory we made above.
COPY . .

# Use the `deps` target to install all of our package dependencies.
# Alternatively, you can use `go get ./...` or `go mod download` (1.14+).
RUN make deps

# Compile the application and place it in the `/go/bin/linksrus-monolith` directory.
# We grab the short GIT SHA of the current commit and inject into the `main.appSha` variable using the -X flag.
#   We use this value when logging so that users know which version of the application is running.
# CGO_ENABLED=0 tells the compiler that we aren't invoking any C code. This allows the compiler to optimize away some code from the final binary.
# -static tells the compiler to produce a static binary.
# -w and -s will drop some debug signals, meaning you won't be able to attach a debugger to this binary (but we don't need it for production).
RUN GIT_SHA=$(git rev-parse --short HEAD) && \
    CGO_ENABLED=0 GOARCH=amd64 GOOS=linux \
    go build -a \
    -ldflags "-extldflags '-static' -w -s -X main.appSha=$GIT_SHA" \
    -o /go/bin/linksrus-monolith \
    $PATH_TO_APP
# Note: change the above path to your own application.

# Build the final container that will contain only the finished binary and any other dependencies.
FROM alpine:3.10

# Our application will be making TLS connections, so we need to download the global TLS certificates.
RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*

# Copy over our binary from the builder container.
COPY --from=builder /go/bin/linksrus-monolith /go/bin/linksrus-monolith

# Launch our binary on startup.
ENTRYPOINT ["/go/bin/linksrus-monolith"]
