# Build the manager binary
FROM golang:1.15 as builder
ARG GITLAB_TOKEN

# install and configure git. we need it because some of our modules
# (e.g., egw-resource-model) are private
RUN git config --global url."https://oauth2:${GITLAB_TOKEN}@gitlab.com/acnodal".insteadOf https://gitlab.com/acnodal

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

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager main.go

FROM ubuntu:20.04
ENV DEST=/opt/acnodal/bin
ENV PROGRAM=${DEST}/manager

RUN apt-get update && apt-get install -y ipset iptables iproute2

# copy executables from the builder image
COPY --from=builder /workspace/manager ${PROGRAM}

# copy the packet forwarding components from the pfc dockerfile
COPY --from=registry.gitlab.com/acnodal/packet-forwarding-component/pfc:latest /opt/acnodal/bin/* ${DEST}/

# FIXME: run ${PROGRAM} instead of this hard-coded string
ENTRYPOINT ["/opt/acnodal/bin/manager"]
