package codexruntime

import (
	"reflect"
	"testing"
)

func TestApprovalBridge(t *testing.T) {
	t.Parallel()

	t.Run("command approval accepts", func(t *testing.T) {
		t.Parallel()

		got, err := handleApprovalRequest(MethodItemCommandExecutionRequestApproval, CommandExecutionApprovalRequestParams{})
		if err != nil {
			t.Fatalf("handleApprovalRequest() error = %v", err)
		}

		want := map[string]any{"decision": "accept"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("handleApprovalRequest() = %#v, want %#v", got, want)
		}
	})

	t.Run("file change approval accepts", func(t *testing.T) {
		t.Parallel()

		got, err := handleApprovalRequest(MethodItemFileChangeRequestApproval, FileChangeApprovalRequestParams{})
		if err != nil {
			t.Fatalf("handleApprovalRequest() error = %v", err)
		}

		want := map[string]any{"decision": "accept"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("handleApprovalRequest() = %#v, want %#v", got, want)
		}
	})

	t.Run("permissions approval grants requested permissions for turn", func(t *testing.T) {
		t.Parallel()

		requested := map[string]any{
			"filesystem": "write",
			"network":    "enabled",
		}

		got, err := handleApprovalRequest(MethodItemPermissionsRequestApproval, PermissionsApprovalRequestParams{
			RequestedPermissions: requested,
		})
		if err != nil {
			t.Fatalf("handleApprovalRequest() error = %v", err)
		}

		want := map[string]any{
			"permissions": requested,
			"scope":       "turn",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("handleApprovalRequest() = %#v, want %#v", got, want)
		}
	})
}
