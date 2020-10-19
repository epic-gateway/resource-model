FROM golang:alpine as builder

ENV GOOS=linux
ARG GITLAB_TOKEN

# install and configure git. we need it because some of our modules
# (e.g., egw-resource-model) are private
RUN apk add git
RUN git config --global url."https://oauth2:${GITLAB_TOKEN}@gitlab.com/acnodal".insteadOf https://gitlab.com/acnodal

WORKDIR /opt/acnodal/src
COPY . ./

# build the egw operator (static)
RUN go build  -tags 'osusergo netgo' ./main.go


# start fresh
FROM alpine:3
ENV DEST=/opt/acnodal/bin
ENV PROGRAM=${DEST}/operator

RUN apk add ipset

# copy executables from the builder image
COPY --from=builder /opt/acnodal/src/main ${PROGRAM}

# copy the packet forwarding components from the pfc dockerfile
COPY --from=registry.gitlab.com/acnodal/packet-forwarding-component/pfc:latest /tmp/.acnodal/bin/* ${DEST}/

EXPOSE 8081

# FIXME: run ${PROGRAM} instead of this hard-coded string
CMD ["/opt/acnodal/bin/operator"]
