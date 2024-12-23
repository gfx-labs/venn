package version

import (
	"fmt"
	"runtime/debug"
)

var Commit = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}

	return ""
}()

var Version = fmt.Sprintf("%s_%s+%s", "venn", "x.x.x", Commit)
