FROM golang:alpine as builder

ENV GOOS=linux
ARG GITLAB_TOKEN

# install and configure git. we need it because some of our modules
# (e.g., egw-resource-model) are private
RUN apk add git
RUN git config --global url."https://oauth2:${GITLAB_TOKEN}@gitlab.com/acnodal".insteadOf https://gitlab.com/acnodal

WORKDIR /usr/src/xds-operator
COPY . ./

# build the xDS operator (static)
RUN go build  -tags 'osusergo netgo' -o /usr/local/bin/xds-operator ./main.go


# start fresh
FROM ubuntu:20.04

# install dependencies
RUN apt-get update \
    && apt-get install -y libelf-dev \
    && rm -rf /var/lib/apt/lists/*

# copy executables from the builder image
WORKDIR /usr/local/bin
COPY --from=builder /usr/local/bin/xds-operator ./

EXPOSE 8080
CMD ["/usr/local/bin/xds-operator", "--debug"]
