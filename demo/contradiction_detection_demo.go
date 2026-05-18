// demo/contradiction_detection_demo.go demonstrates contradiction detection:
// a provider reports success but the observed resource state contradicts it.
//
// Run with: go run ./demo/contradiction_detection_demo.go
package main

import (
	"fmt"

	"github.com/janlauber/one-click/pkg/providerops"
)

func main() {
	fmt.Println("=== AutoStack Contradiction Detection Demo ===")
	fmt.Println()

	cases := []struct {
		label  string
		input  providerops.ContradictionInput
	}{
		{
			label: "Deploy reported success but resource is missing",
			input: providerops.ContradictionInput{
				OperationID:    "op-001",
				OperationType:  providerops.OperationTypeDeploy,
				ReportedResult: providerops.ResultStateSuccess,
				ResourceExists: false, // contradiction: success but resource is gone
			},
		},
		{
			label: "Delete reported success but resource is still alive",
			input: providerops.ContradictionInput{
				OperationID:    "op-002",
				OperationType:  providerops.OperationTypeDestroy,
				ReportedResult: providerops.ResultStateSuccess,
				ResourceExists: true, // contradiction: delete succeeded but resource persists
			},
		},
		{
			label: "Scale acknowledged but capacity unchanged",
			input: providerops.ContradictionInput{
				OperationID:       "op-003",
				OperationType:     providerops.OperationTypeScale,
				ReportedResult:    providerops.ResultStateSuccess,
				RequestedCapacity: 5,
				ObservedCapacity:  2, // contradiction: scale said ok but capacity didn't change
			},
		},
		{
			label: "Deploy completed normally (no contradiction)",
			input: providerops.ContradictionInput{
				OperationID:    "op-004",
				OperationType:  providerops.OperationTypeDeploy,
				ReportedResult: providerops.ResultStateSuccess,
				ResourceExists: true, // all good
			},
		},
	}

	for i, tc := range cases {
		fmt.Printf("[%d] %s\n", i+1, tc.label)
		c, err := providerops.DetectProviderContradiction(
			fmt.Sprintf("contradiction-%d", i+1),
			"exec-demo-001", "tenant-demo", "aws",
			tc.input,
		)
		if err != nil {
			fmt.Printf("    Detection error: %v\n", err)
			continue
		}
		if c == nil {
			fmt.Printf("    No contradiction detected — operation is consistent.\n")
		} else {
			fmt.Printf("    ✗ CONTRADICTION DETECTED\n")
			fmt.Printf("    Type: %s\n", c.Type)
			fmt.Printf("    Impact: %.2f confidence reduction\n", c.ConfidenceImpact)
			fmt.Printf("    Operator action required: investigate before retry\n")
		}
		fmt.Println()
	}

	fmt.Println("Key guarantee: contradictions are NEVER auto-resolved.")
	fmt.Println("The operator decides what happens next.")
}
