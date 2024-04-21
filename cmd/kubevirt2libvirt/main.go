// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfield "k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/utils/cpuset"
	virtv1 "kubevirt.io/api/core/v1"
	instancetypev1beta1 "kubevirt.io/api/instancetype/v1beta1"
	cmdv1 "kubevirt.io/kubevirt/pkg/handler-launcher-com/cmd/v1"
	"kubevirt.io/kubevirt/pkg/instancetype"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/converter"
)

func main() {
	app := &cli.App{
		Usage: "Convert KubeVirt YAML into libvirt XML",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "preferences",
			},
			&cli.StringFlag{
				Name: "instancetypes",
			},
			&cli.StringFlag{
				Name: "cpuset",
			},
		},
		Action: cli.ActionFunc(run),
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%+v", err)
		os.Exit(1)
	}
}

func run(c *cli.Context) error {
	err := instancetypev1beta1.AddToScheme(scheme.Scheme)
	if err != nil {
		return err
	}

	if file := c.String("preferences"); file != "" {
		objs, err := decodeObjects(file, []schema.GroupVersionKind{
			instancetypev1beta1.SchemeGroupVersion.WithKind("VirtualMachinePreference"),
			instancetypev1beta1.SchemeGroupVersion.WithKind("VirtualMachineClusterPreference"),
		})
		if err != nil {
			return err
		}

		for _, obj := range objs {
			if preference, ok := obj.(*instancetypev1beta1.VirtualMachinePreference); ok {
				knownPreferences[preference.Name] = &preference.Spec
			} else {
				preference := obj.(*instancetypev1beta1.VirtualMachineClusterPreference)
				knownPreferences[preference.Name] = &preference.Spec
			}
		}
	}

	if file := c.String("instancetypes"); file != "" {
		objs, err := decodeObjects(file, []schema.GroupVersionKind{
			instancetypev1beta1.SchemeGroupVersion.WithKind("VirtualMachineInstancetype"),
			instancetypev1beta1.SchemeGroupVersion.WithKind("VirtualMachineClusterInstancetype"),
		})
		if err != nil {
			return err
		}

		for _, obj := range objs {
			if instancetype, ok := obj.(*instancetypev1beta1.VirtualMachineInstancetype); ok {
				knownInstancetypes[instancetype.Name] = &instancetype.Spec
			} else {
				instancetype := obj.(*instancetypev1beta1.VirtualMachineClusterInstancetype)
				knownInstancetypes[instancetype.Name] = &instancetype.Spec
			}
		}
	}

	var cpuSet []int
	if cpuSetString := c.String("cpuset"); cpuSetString != "" {
		parsed, err := cpuset.Parse(c.Args().Get(0))
		if err != nil {
			return err
		}

		cpuSet = parsed.List()
	}

	return convert(cpuSet)
}

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

func contains[T comparable](slice []T, elem T) bool {
	for _, e := range slice {
		if e == elem {
			return true
		}
	}
	return false
}

var knownPreferences = map[string]*instancetypev1beta1.VirtualMachinePreferenceSpec{}
var knownInstancetypes = map[string]*instancetypev1beta1.VirtualMachineInstancetypeSpec{}

func convert(cpuSet []int) error {
	// read YAML from stdin

	vmYaml, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	// unmarshal VM

	vm := &virtv1.VirtualMachine{}
	err = yaml.Unmarshal(vmYaml, vm)
	if err != nil {
		return err
	}

	// look up preference and instancetype

	var preferenceSpec *instancetypev1beta1.VirtualMachinePreferenceSpec
	var instancetypeSpec *instancetypev1beta1.VirtualMachineInstancetypeSpec

	if vm.Spec.Preference != nil {
		var ok bool
		if preferenceSpec, ok = knownPreferences[vm.Spec.Preference.Name]; !ok {
			return fmt.Errorf("unknown %s \"%s\"", vm.Spec.Preference.Kind, vm.Spec.Preference.Name)
		}
	}

	if vm.Spec.Instancetype != nil {
		var ok bool
		if instancetypeSpec, ok = knownInstancetypes[vm.Spec.Instancetype.Name]; !ok {
			return fmt.Errorf("unknown %s \"%s\"", vm.Spec.Instancetype.Kind, vm.Spec.Instancetype.Name)
		}
	}

	// generate VMI

	vmi := &virtv1.VirtualMachineInstance{}
	vmi.APIVersion = virtv1.GroupVersion.String()
	vmi.Kind = "VirtualMachineInstance"

	if vm.Spec.Template != nil {
		vmi.Labels = vm.Spec.Template.ObjectMeta.Labels
		vmi.Spec = *vm.Spec.Template.Spec.DeepCopy()
	}

	// apply preference

	if preferenceSpec != nil {
		instancetype.ApplyDevicePreferences(preferenceSpec, &vmi.Spec)
	}

	// apply instancetype

	instancetypeMethods := &instancetype.InstancetypeMethods{}
	conflicts := instancetypeMethods.ApplyToVmi(
		k8sfield.NewPath("spec", "template", "spec"),
		instancetypeSpec, preferenceSpec, &vmi.Spec, &vmi.ObjectMeta)
	if len(conflicts) > 0 {
		return fmt.Errorf("instancetype conflicts: %+v", conflicts)
	}

	// convert to domain

	numaCell := &cmdv1.Cell{}
	for _, cpu := range cpuSet {
		numaCell.Cpus = append(numaCell.Cpus, &cmdv1.CPU{Id: uint32(cpu)})
	}

	context := &converter.ConverterContext{
		CPUSet:           cpuSet,
		VirtualMachine:   vmi,
		EFIConfiguration: &converter.EFIConfiguration{},
		Topology: &cmdv1.Topology{
			NumaCells: []*cmdv1.Cell{numaCell},
		},
	}

	domain := &api.Domain{}
	err = converter.Convert_v1_VirtualMachineInstance_To_api_Domain(vmi, domain, context)
	if err != nil {
		return err
	}

	// marshal domain

	domainXml, err := xml.MarshalIndent(domain.Spec, "", "  ")
	if err != nil {
		return err
	}

	// write XML to stdout

	fmt.Printf("%s\n", domainXml)
	return nil
}
