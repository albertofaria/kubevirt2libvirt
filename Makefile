# SPDX-License-Identifier: Apache-2.0

.PHONY: build
build:
	go build -o bin/kubevirt2libvirt ./cmd/kubevirt2libvirt

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...
