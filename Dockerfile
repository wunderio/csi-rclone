####
FROM golang:alpine AS builder
RUN apk update && apk add --no-cache git make bash
WORKDIR $GOPATH/src/csi-rclone-nodeplugin
COPY . .
RUN make plugin

####
FROM alpine:3.9
RUN apk add --no-cache ca-certificates bash fuse curl unzip

RUN curl https://rclone.org/install.sh | bash

COPY --from=builder /go/src/csi-rclone-nodeplugin/_output/csi-rclone-plugin /bin/csi-rclone-plugin
ENTRYPOINT ["/bin/csi-rclone-plugin"]
