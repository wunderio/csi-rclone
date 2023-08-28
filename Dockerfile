####
FROM golang:alpine AS builder
RUN apk update && apk add --no-cache git make bash
WORKDIR $GOPATH/src/csi-rclone-nodeplugin
COPY . .
RUN make plugin

####
FROM alpine:3.16
RUN apk add --no-cache ca-certificates bash fuse3 curl unzip tini

RUN curl https://rclone.org/install.sh | bash

# Use pre-compiled version (with cirectory marker patch)
# https://github.com/rclone/rclone/pull/5323
# COPY bin/rclone /usr/bin/rclone
# RUN chmod 755 /usr/bin/rclone \
#     && chown root:root /usr/bin/rclone

COPY --from=builder /go/src/csi-rclone-nodeplugin/_output/csi-rclone-plugin /bin/csi-rclone-plugin

ENTRYPOINT [ "/sbin/tini", "--"]
CMD ["/bin/csi-rclone-plugin"]
