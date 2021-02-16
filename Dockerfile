# Build the manager binary
FROM golang:1.15 as builder
ARG GITLAB_AUTHN

RUN git config --global url."https://$GITLAB_AUTHN@gitlab.com/acnodal".insteadOf https://gitlab.com/acnodal

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY internal/ internal/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager main.go

FROM ubuntu:20.04

RUN apt-get update && apt-get install -y curl ipset iptables iproute2 linux-tools-generic

COPY --from=builder /workspace/manager /usr/local/bin/manager
CMD [ "/usr/local/bin/manager" ]
