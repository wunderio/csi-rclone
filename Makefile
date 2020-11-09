# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

VERSION=$(shell cat VERSION)
REGISTRY_NAME=dippynark
IMAGE_NAME=csi-rclone
IMAGE_TAG=$(REGISTRY_NAME)/$(IMAGE_NAME):$(VERSION)

.PHONY: all rclone-plugin clean rclone-container

all: plugin container push

plugin:
	go mod download
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-X github.com/wunderio/csi-rclone/pkg/rclone.DriverVersion=$(VERSION) -extldflags "-static"' -o _output/csi-rclone-plugin ./cmd/csi-rclone-plugin
	
container:
	docker build -t $(IMAGE_TAG) -f ./cmd/csi-rclone-plugin/Dockerfile .

push:
	docker push $(IMAGE_TAG)
clean:
	go clean -r -x
	-rm -rf _output
