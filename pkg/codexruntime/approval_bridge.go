package codexruntime

import "fmt"

const approvalPolicyPermanentYOLO = "never"

func handleApprovalRequest(method string, params any) (map[string]any, error) {
	switch method {
	case MethodItemCommandExecutionRequestApproval, MethodItemFileChangeRequestApproval:
		return map[string]any{"decision": "accept"}, nil
	case MethodItemPermissionsRequestApproval:
		request, ok := params.(PermissionsApprovalRequestParams)
		if !ok {
			return nil, fmt.Errorf("codexruntime: unexpected params type for %s", method)
		}
		return map[string]any{
			"permissions": request.RequestedPermissions,
			"scope":       "turn",
		}, nil
	default:
		return nil, fmt.Errorf("codexruntime: unsupported approval request %s", method)
	}
}
