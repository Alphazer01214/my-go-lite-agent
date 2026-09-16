package serve

import (
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

// debugOn gates Host debug logging. Enabled by -debug on the CLI entries.
// Logs go to stderr so they never mix with Render Medium stdout.
var debugOn atomic.Bool

// SetDebug enables or disables Host debug logging (Frame traffic and lifecycle).
func SetDebug(on bool) {
	debugOn.Store(on)
}

func debugf(format string, args ...any) {
	if !debugOn.Load() {
		return
	}
	fmt.Fprintf(os.Stderr, "[debug] %s %s\n",
		time.Now().Format("15:04:05.000"),
		fmt.Sprintf(format, args...),
	)
}

// debugPayloadLimit keeps Frame payloads readable in a terminal.
const debugPayloadLimit = 400

func debugFrame(dir, plugin string, f *protocol.Frame) {
	if !debugOn.Load() || f == nil {
		return
	}
	payload, n := truncateDebugPayload(f.Payload, debugPayloadLimit)
	errStr := ""
	if f.Error != nil {
		errStr = fmt.Sprintf(" error=%s", f.Error.Error())
	}
	debugf("%s plugin=%s id=%s type=%s cap=%s method=%s bytes=%d payload=%s%s",
		dir, plugin, f.ID, f.Type, f.Cap, f.Method, n, payload, errStr)
}

func truncateDebugPayload(raw json.RawMessage, max int) (string, int) {
	n := len(raw)
	if n == 0 {
		return "-", 0
	}
	s := string(raw)
	if n <= max {
		return s, n
	}
	return s[:max] + fmt.Sprintf("…(+%d)", n-max), n
}
