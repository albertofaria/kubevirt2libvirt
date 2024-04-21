// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/converter"
	"sigs.k8s.io/yaml"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s\n", os.Args[0])
		os.Exit(2)
	}

	err := convert()
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
}

func convert() error {
	vmiYaml, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	vmi := &v1.VirtualMachineInstance{}
	err = yaml.Unmarshal(vmiYaml, vmi, yaml.DisallowUnknownFields)
	if err != nil {
		return err
	}

	domain := &api.Domain{}
	context := &converter.ConverterContext{}

	err = converter.Convert_v1_VirtualMachineInstance_To_api_Domain(vmi, domain, context)
	if err != nil {
		return err
	}

	domainXml, err := xml.MarshalIndent(domain.Spec, "", "  ")
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", domainXml)
	return nil
}
