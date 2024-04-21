#!/bin/bash
# SPDX-License-Identifier: Apache-2.0

set -o errexit -o pipefail -o nounset

url=https://github.com/kubevirt/common-instancetypes.git
tag=v1.0.0

kubectl kustomize --output common-instancetypes.yaml \
    "$url/VirtualMachineInstancetypes?ref=$tag"

kubectl kustomize --output common-preferences.yaml \
    "$url/VirtualMachinePreferences?ref=$tag"
