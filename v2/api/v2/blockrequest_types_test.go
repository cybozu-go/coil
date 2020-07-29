package v2

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestBlockRequestGetResult(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		status    BlockRequestStatus
		expectErr bool
		blockName string
	}{
		{
			"completed",
			BlockRequestStatus{
				AddressBlockName: "foo",
				Conditions: []BlockRequestCondition{
					{Type: BlockRequestComplete, Status: corev1.ConditionTrue},
				},
			},
			false,
			"foo",
		},
		{
			"completed-false-failure",
			BlockRequestStatus{
				AddressBlockName: "foo",
				Conditions: []BlockRequestCondition{
					{Type: BlockRequestFailed, Status: corev1.ConditionFalse},
					{Type: BlockRequestComplete, Status: corev1.ConditionTrue},
				},
			},
			false,
			"foo",
		},
		{
			"completed-failure",
			BlockRequestStatus{
				AddressBlockName: "foo",
				Conditions: []BlockRequestCondition{
					{Type: BlockRequestComplete, Status: corev1.ConditionTrue},
					{Type: BlockRequestFailed, Status: corev1.ConditionTrue},
				},
			},
			true,
			"foo",
		},
		{
			"not-completed",
			BlockRequestStatus{
				AddressBlockName: "foo",
				Conditions: []BlockRequestCondition{
					{Type: BlockRequestComplete, Status: corev1.ConditionFalse},
				},
			},
			true,
			"foo",
		},
		{
			"no-conditions",
			BlockRequestStatus{
				AddressBlockName: "foo",
			},
			true,
			"foo",
		},
		{
			"failed",
			BlockRequestStatus{
				AddressBlockName: "foo",
				Conditions: []BlockRequestCondition{
					{Type: BlockRequestFailed, Status: corev1.ConditionTrue},
				},
			},
			true,
			"foo",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.status.getResult()
			if err != nil {
				if !tc.expectErr {
					t.Error("unexpected error", err)
				}
				return
			}
			if actual != tc.blockName {
				t.Error("unexpected block name", actual)
			}
		})
	}
}
