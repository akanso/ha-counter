# Build the manager binary
FROM golang:1.17.2 as builder

# Copy in the go src
WORKDIR /go-code/src/go-counter
COPY src/    src/
COPY go.mod  go.mod 
COPY go.sum go.sum
#COPY vendor/go.etcd.io     /go-code/src/go.etcd.io
#COPY vendor/github.com/    /go-code/src/github.com/

# Build
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o counter ./src/counter-app.go

# Copy the controller-manager into a thin image
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /go-code/src/go-counter .
ENTRYPOINT ["/counter"]
