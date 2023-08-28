FROM alpine:3.16
RUN apk add --no-cache ca-certificates bash fuse curl unzip tini

# RUN curl https://rclone.org/install.sh | bash

# Use pre-compiled version (with cirectory marker patch)
# https://github.com/rclone/rclone/pull/5323
COPY ./install-dm.sh /tmp
COPY ./rclone-build /tmp/rclone-build
RUN /tmp/install-dm.sh

COPY ./_output/csi-rclone-plugin-dm /bin/csi-rclone-plugin

ENTRYPOINT [ "/sbin/tini", "--"]
CMD ["/bin/csi-rclone-plugin"]