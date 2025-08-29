FROM golang:1.25-alpine as builder

COPY . /go/src/github.com/ucphhpc/docker-volume-sshfs
WORKDIR /go/src/github.com/ucphhpc/docker-volume-sshfs

RUN set -ex \
    && apk add --no-cache --virtual .build-deps \
    gcc libc-dev \
    && go install --ldflags '-extldflags "-static"' \
    && apk del .build-deps

CMD ["/go/bin/docker-volume-sshfs"]

FROM alpine
RUN apk update && apk add sshfs
RUN mkdir -p /run/docker/plugins /mnt/state /mnt/volumes
COPY --from=builder /go/bin/docker-volume-sshfs .
# Tini to reap orphaned child procceses
# Add Tini
RUN apk add tini
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["docker-volume-sshfs"]
