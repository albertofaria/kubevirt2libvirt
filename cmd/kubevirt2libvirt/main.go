// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli"
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
		Name:  "kubevirt2libvirt",
		Usage: "Convert KubeVirt YAML into libvirt XML",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "cpuset"},
			&cli.StringFlag{Name: "preferences"},
			&cli.StringFlag{Name: "instancetypes"},
		},
		Action: cli.ActionFunc(run),
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%+v", err)
		os.Exit(1)
	}
}

type options struct {
	cpuSet             []int
	knownPreferences   map[string]*instancetypev1beta1.VirtualMachinePreferenceSpec
	knownInstancetypes map[string]*instancetypev1beta1.VirtualMachineInstancetypeSpec
}

func parseOptions(c *cli.Context) (*options, error) {
	opts := &options{
		cpuSet:             []int{},
		knownPreferences:   map[string]*instancetypev1beta1.VirtualMachinePreferenceSpec{},
		knownInstancetypes: map[string]*instancetypev1beta1.VirtualMachineInstancetypeSpec{},
	}

	if cpuSetString := c.String("cpuset"); cpuSetString != "" {
		parsed, err := cpuset.Parse(c.Args().Get(0))
		if err != nil {
			return nil, err
		}

		opts.cpuSet = parsed.List()
	}

	if file := c.String("preferences"); file != "" {
		objs, err := decodeObjects(file, []schema.GroupVersionKind{
			instancetypev1beta1.SchemeGroupVersion.WithKind("VirtualMachinePreference"),
			instancetypev1beta1.SchemeGroupVersion.WithKind("VirtualMachineClusterPreference"),
		})
		if err != nil {
			return nil, err
		}

		for _, obj := range objs {
			if preference, ok := obj.(*instancetypev1beta1.VirtualMachinePreference); ok {
				opts.knownPreferences[preference.Name] = &preference.Spec
			} else {
				preference := obj.(*instancetypev1beta1.VirtualMachineClusterPreference)
				opts.knownPreferences[preference.Name] = &preference.Spec
			}
		}
	}

	if file := c.String("instancetypes"); file != "" {
		objs, err := decodeObjects(file, []schema.GroupVersionKind{
			instancetypev1beta1.SchemeGroupVersion.WithKind("VirtualMachineInstancetype"),
			instancetypev1beta1.SchemeGroupVersion.WithKind("VirtualMachineClusterInstancetype"),
		})
		if err != nil {
			return nil, err
		}

		for _, obj := range objs {
			if instancetype, ok := obj.(*instancetypev1beta1.VirtualMachineInstancetype); ok {
				opts.knownInstancetypes[instancetype.Name] = &instancetype.Spec
			} else {
				instancetype := obj.(*instancetypev1beta1.VirtualMachineClusterInstancetype)
				opts.knownInstancetypes[instancetype.Name] = &instancetype.Spec
			}
		}
	}

	return opts, nil
}

func run(c *cli.Context) error {
	opts, err := parseOptions(c)
	if err != nil {
		return err
	}

	yaml, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	xml, err := convert(yaml, opts)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", xml)

	return nil
}

func convert(yaml []byte, opts *options) ([]byte, error) {
	// unmarshal VM

	obj, gvk, err := scheme.Codecs.UniversalDeserializer().Decode(yaml, nil, nil)
	if err != nil {
		return nil, err
	}

	var vm *virtv1.VirtualMachine

	switch *gvk {
	case virtv1.VirtualMachineGroupVersionKind:
		vm = obj.(*virtv1.VirtualMachine)
	case virtv1.VirtualMachineInstanceGroupVersionKind:
		vmi := obj.(*virtv1.VirtualMachineInstance)
		vm = &virtv1.VirtualMachine{
			Spec: virtv1.VirtualMachineSpec{
				Template: &virtv1.VirtualMachineInstanceTemplateSpec{
					Spec: vmi.Spec,
				},
			},
		}
	default:
		return nil, fmt.Errorf("unsupported object %s", gvk)
	}

	// look up preference and instancetype

	var preferenceSpec *instancetypev1beta1.VirtualMachinePreferenceSpec
	var instancetypeSpec *instancetypev1beta1.VirtualMachineInstancetypeSpec

	if vm.Spec.Preference != nil {
		var ok bool
		if preferenceSpec, ok = opts.knownPreferences[vm.Spec.Preference.Name]; !ok {
			return nil, fmt.Errorf("unknown %s \"%s\"", vm.Spec.Preference.Kind, vm.Spec.Preference.Name)
		}
	}

	if vm.Spec.Instancetype != nil {
		var ok bool
		if instancetypeSpec, ok = opts.knownInstancetypes[vm.Spec.Instancetype.Name]; !ok {
			return nil, fmt.Errorf("unknown %s \"%s\"", vm.Spec.Instancetype.Kind, vm.Spec.Instancetype.Name)
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
		return nil, fmt.Errorf("instancetype conflicts: %+v", conflicts)
	}

	// convert to domain

	numaCell := &cmdv1.Cell{}
	for _, cpu := range opts.cpuSet {
		numaCell.Cpus = append(numaCell.Cpus, &cmdv1.CPU{Id: uint32(cpu)})
	}

	context := &converter.ConverterContext{
		CPUSet:           opts.cpuSet,
		VirtualMachine:   vmi,
		EFIConfiguration: &converter.EFIConfiguration{},
		Topology: &cmdv1.Topology{
			NumaCells: []*cmdv1.Cell{numaCell},
		},
	}

	domain := &api.Domain{}
	err = converter.Convert_v1_VirtualMachineInstance_To_api_Domain(vmi, domain, context)
	if err != nil {
		return nil, err
	}

	// marshal domain

	return xml.MarshalIndent(domain.Spec, "", "  ")
}

func init() {
	virtv1.AddToScheme(scheme.Scheme)
	instancetypev1beta1.AddToScheme(scheme.Scheme)
}
