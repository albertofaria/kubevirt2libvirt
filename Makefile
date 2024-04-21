# SPDX-License-Identifier: Apache-2.0

prefix ?= /usr

.PHONY: build
build:
	go build -o bin/kubevirt2libvirt ./cmd/kubevirt2libvirt

.PHONY: build
install: build
	install -D -m 0755 -t $(DESTDIR)$(prefix)/bin bin/kubevirt2libvirt

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: lint
lint:
	gofmt -l .
	go vet ./...
