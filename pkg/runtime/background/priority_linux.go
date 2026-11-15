package background

import (
	"log/slog"

	"golang.org/x/sys/unix"
)

func lower(nice int) {
	if err := unix.Setpriority(unix.PRIO_PROCESS, unix.Gettid(), nice); err != nil {
		slog.Warn("lowering a background thread's priority failed", "nice", nice, "err", err)
	}
}
