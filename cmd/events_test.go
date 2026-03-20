package cmd

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

func TestFilterFailedEvents(t *testing.T) {
	events := []types.StackEvent{
		{ResourceStatus: types.ResourceStatusCreateComplete, LogicalResourceId: strPtr("VPC")},
		{ResourceStatus: types.ResourceStatusCreateFailed, LogicalResourceId: strPtr("Subnet")},
		{ResourceStatus: types.ResourceStatusUpdateComplete, LogicalResourceId: strPtr("SG")},
		{ResourceStatus: types.ResourceStatusUpdateFailed, LogicalResourceId: strPtr("IGW")},
		{ResourceStatus: types.ResourceStatusDeleteFailed, LogicalResourceId: strPtr("EIP")},
		{ResourceStatus: types.ResourceStatusDeleteComplete, LogicalResourceId: strPtr("NAT")},
	}

	filtered := filterFailedEvents(events)

	if len(filtered) != 3 {
		t.Fatalf("expected 3 failed events, got %d", len(filtered))
	}

	wantIDs := []string{"Subnet", "IGW", "EIP"}
	for i, want := range wantIDs {
		got := getValue(filtered[i].LogicalResourceId)
		if got != want {
			t.Errorf("filtered[%d] LogicalResourceId = %q, want %q", i, got, want)
		}
	}
}

func TestFilterFailedEvents_NoFailures(t *testing.T) {
	events := []types.StackEvent{
		{ResourceStatus: types.ResourceStatusCreateComplete},
		{ResourceStatus: types.ResourceStatusUpdateComplete},
	}

	filtered := filterFailedEvents(events)
	if len(filtered) != 0 {
		t.Errorf("expected 0 failed events, got %d", len(filtered))
	}
}

func TestFilterFailedEvents_Empty(t *testing.T) {
	filtered := filterFailedEvents(nil)
	if len(filtered) != 0 {
		t.Errorf("expected 0 failed events, got %d", len(filtered))
	}
}
