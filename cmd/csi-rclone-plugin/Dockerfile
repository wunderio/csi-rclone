FROM alpine:3.9
RUN apk add --no-cache ca-certificates bash fuse curl unzip

RUN curl https://rclone.org/install.sh | bash

COPY ./_output/csi-rclone-plugin /bin/csi-rclone-plugin
ENTRYPOINT ["/bin/csi-rclone-plugin"]
