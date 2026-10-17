package host

import "encoding/json"

// UIPublisher accepts trusted plugin UI publishes from Host methods.
// The publisher validates Mount ownership and queues browser delivery.
type UIPublisher interface {
	PublishState(from string, payload json.RawMessage) (json.RawMessage, error)
	PublishCommand(from string, payload json.RawMessage) (json.RawMessage, error)
}
