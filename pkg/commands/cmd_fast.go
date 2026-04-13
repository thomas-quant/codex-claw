package commands

import "context"

func fastCommand() Definition {
	return Definition{
		Name:        "fast",
		Description: "Toggle fast mode",
		Usage:       "/fast",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.ToggleFast == nil {
				return req.Reply(unavailableMsg)
			}
			enabled, err := runtimeToggleFast(rt)
			if err != nil {
				return req.Reply(err.Error())
			}
			if enabled {
				return req.Reply("Fast mode enabled")
			}
			return req.Reply("Fast mode disabled")
		},
	}
}
