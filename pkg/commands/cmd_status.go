package commands

import "context"

func statusCommand() Definition {
	return Definition{
		Name:        "status",
		Description: "Show per-thread runtime status",
		Usage:       "/status",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.ReadStatus == nil {
				return req.Reply(unavailableMsg)
			}
			status, ok := runtimeReadStatus(rt)
			if !ok {
				return req.Reply(unavailableMsg)
			}
			return req.Reply(formatStatusSnapshot(status))
		},
	}
}
