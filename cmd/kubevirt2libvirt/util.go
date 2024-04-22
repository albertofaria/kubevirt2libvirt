// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kubectl/pkg/scheme"
)

func contains[T comparable](slice []T, elem T) bool {
	for _, e := range slice {
		if e == elem {
			return true
		}
	}
	return false
}

// Returns a list of objects defined in the given single- or multi-document YAML file.
func decodeObjects(path string, allowedGvks []schema.GroupVersionKind) ([]runtime.Object, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var objects []runtime.Object
	decoder := yaml.NewDecoder(file)

	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			} else {
				return nil, err
			}
		}

		content, err := yaml.Marshal(&node)
		if err != nil {
			return nil, err
		}

		obj, gvk, err := scheme.Codecs.UniversalDeserializer().Decode(content, nil, nil)
		if err != nil {
			return nil, err
		}

		if !contains(allowedGvks, *gvk) {
			return nil, fmt.Errorf("unexpected kind")
		}

		objects = append(objects, obj)
	}

	return objects, nil
}
