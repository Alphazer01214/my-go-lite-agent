package pluginsdk

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/tomori/my-go-lite-agent/internal/host"
)

type Request struct {
	ID         string
	Capability string
	Method     string
	Payload    json.RawMessage
}

type Plugin struct {
	mu   sync.Mutex
	name string
	// handlers maps "cap.method" to a HandleFunc
	handlers map[string]HandleFunc
	pending  map[string]chan *host.Frame
	stdin    *os.File
	stdout   *os.File

	seq
}

// HandleFunc plugin implements this to process a task
type HandleFunc func(req *Request) (json.RawMessage, error)

func NewPlugin(name string) *Plugin {
	return &Plugin{
		name:     name,
		handlers: make(map[string]HandleFunc),
		pending:  make(map[string]chan *host.Frame),
	}
}

func (p *Plugin) Register(capability string, method string, handler HandleFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handlers[capability+"."+method] != nil {
		panic("handler already registered for " + capability + "." + method)
	}
	p.handlers[capability+"."+method] = handler
}

// Call invokes another Capability through Host (star topology). Blocks until res or error.
func (p *Plugin) Call(capability string, method string, payload json.RawMessage) (json.RawMessage, error) {
	return p.callFrame(p.name, capability, method, payload)
}

func (p *Plugin) callFrame(name string, capability string, method string, payload json.RawMessage) (json.RawMessage, error) {

}
