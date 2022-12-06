# Build the manager binary
FROM golang:1.17 as builder

ARG GOPROXY
WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GO111MODULE=on go build -a -o jwt cmd/tools/jwt/jwt_cmd.go

FROM golang:1.17 as tool-builder
ARG GOPROXY
ARG CACHEBUST
RUN echo ${CACHEBUST} && git clone -b bugfix-gc-pipeline https://github.com/yudong2015/ks.git
RUN cd ks && CGO_ENABLED=0 GO111MODULE=on go build -o /usr/local/bin/ks

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM kubesphere/distroless-static:nonroot

COPY --from=builder /workspace/jwt /usr/local/bin/jwt
COPY --from=tool-builder /usr/local/bin/ks /usr/local/bin/ks
USER nonroot:nonroot

CMD ["/jwt"]
