package commands

import "context"

func compactCommand() Definition {
	return Definition{
		Name:        "compact",
		Description: "Compact the current thread",
		Usage:       "/compact",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.CompactThread == nil {
				return req.Reply(unavailableMsg)
			}
			if err := runtimeCompactThread(rt); err != nil {
				return req.Reply(err.Error())
			}
			return req.Reply("Thread compacted.")
		},
	}
}
