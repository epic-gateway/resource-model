FROM golang:alpine as builder

ENV PROGRAM=/usr/local/bin/operator
ENV GOOS=linux
ARG GITLAB_TOKEN

# install and configure git. we need it because some of our modules
# (e.g., egw-resource-model) are private
RUN apk add git
RUN git config --global url."https://oauth2:${GITLAB_TOKEN}@gitlab.com/acnodal".insteadOf https://gitlab.com/acnodal

WORKDIR /usr/src
COPY . ./

# build the egw operator (static)
RUN go build  -tags 'osusergo netgo' -o ${PROGRAM} ./main.go


# start fresh
FROM alpine:3

# copy executables from the builder image
COPY --from=builder /usr/local/bin/operator /usr/local/bin

EXPOSE 8080

# FIXME: run ${PROGRAM} instead of this hard-coded string
CMD ["/usr/local/bin/operator"]
