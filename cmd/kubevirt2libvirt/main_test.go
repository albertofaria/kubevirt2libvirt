// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	instancetypev1beta1 "kubevirt.io/api/instancetype/v1beta1"
)

func TestConvert(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "*.in"))
	if err != nil {
		t.Fatal(err)
	}

	opts := &options{
		cpuSet:             []int{},
		knownPreferences:   map[string]*instancetypev1beta1.VirtualMachinePreferenceSpec{},
		knownInstancetypes: map[string]*instancetypev1beta1.VirtualMachineInstancetypeSpec{},
	}

	for _, inPath := range paths {
		inFileName := filepath.Base(inPath)
		testName := inFileName[:len(inFileName)-len(filepath.Ext(inFileName))]
		outPath := filepath.Join(filepath.Dir(inPath), testName+".out")

		t.Run(testName, func(t *testing.T) {
			in, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatal(err)
			}

			expectedOut, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}

			actualOut, err := convert(in, opts)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(expectedOut, append(actualOut, '\n')) {
				t.Fatalf(
					"\n=== INPUT ===\n\n%s\n"+
						"=== EXPECTED OUTPUT ===\n\n%s\n"+
						"=== ACTUAL OUTPUT ===\n\n%s\n",
					in, expectedOut, actualOut,
				)
			}
		})
	}
}
