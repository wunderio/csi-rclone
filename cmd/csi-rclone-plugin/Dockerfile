FROM alpine:3.23
RUN apk add --no-cache ca-certificates bash fuse3 curl unzip tini

# Ubuntu 26.04+ hosts ship a "fusermount3" AppArmor profile attached to /usr/bin/fusermount3 that only
# allows FUSE mounts under home, /mnt, /media, /tmp and /run/user, so mounts into kubelet pod volume dirs are denied.
# rclone looks fusermount3 up via PATH, and /usr/local/bin comes before /usr/bin, so moving it keeps rclone working.
RUN mv /usr/bin/fusermount3 /usr/local/bin/fusermount3

RUN curl https://rclone.org/install.sh | bash

# Use pre-compiled version (with cirectory marker patch)
# https://github.com/rclone/rclone/pull/5323
# COPY bin/rclone /usr/bin/rclone
# RUN chmod 755 /usr/bin/rclone \
#     && chown root:root /usr/bin/rclone

COPY ./_output/csi-rclone-plugin /bin/csi-rclone-plugin

# Disable Multipath TCP listeners (CVE-2026-98432, https://github.com/golang/go/issues/82717)
ENV GODEBUG=multipathtcp=0

ENTRYPOINT [ "/sbin/tini", "--"]
CMD ["/bin/csi-rclone-plugin"]