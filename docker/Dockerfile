from golang:1.14.2-alpine3.11 as builder

arg AGGREGATOR_REPO=https://github.com/monasca/monasca-aggregator
arg AGGREGATOR_BRANCH=master

env GOPATH=/go

run apk add --no-cache \
        git \
        g++ \
        librdkafka-dev \
        libressl-dev \
        make \
        py3-jinja2

run mkdir -p $GOPATH/src/github.com/monasca/monasca-aggregator && \
  cd $GOPATH/src/github.com/monasca/monasca-aggregator && \
  git init && \
  git remote add origin $AGGREGATOR_REPO && \
  git fetch origin $AGGREGATOR_BRANCH && \
  git reset --hard FETCH_HEAD && \
  go build

from alpine:3.11

run apk add --no-cache \
        libressl \
        librdkafka-dev \
        py3-jinja2

copy --from=builder /go/src/github.com/monasca/monasca-aggregator/monasca-aggregator /
copy --from=builder /go/src/github.com/monasca/monasca-aggregator/aggregation-specifications.yaml /specs/aggregation-specifications.yaml

copy template.py start.sh /
copy config.yaml.j2 /config/config.yaml.j2
expose 8080

cmd ["/start.sh"]
