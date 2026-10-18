package protocol

import "errors"

// Stable wire error_code values (protocol.md §7).
// Written to Frame.ErrorCode; SDK uses Code(err) / ErrCode(code, msg) on top.
const (
	CodeHostClosed                 = "host_closed"
	CodeRouteFailed                = "route_failed"
	CodeMethodNotFound             = "method_not_found"
	CodePluginNotMounted           = "plugin_not_mounted"
	CodePluginDown                 = "plugin_down"
	CodePluginDisabled             = "plugin_disabled"
	CodePluginNotReady             = "plugin_not_ready"
	CodeTimeout                    = "timeout"
	CodeCallCancelled              = "call_cancelled"
	CodeCallDeadlineExceeded       = "call_deadline_exceeded"
	CodeConsumePriorityConflict    = "consume_priority_conflict"
	CodeFlowAlreadyProceeded       = "flow_already_proceeded"
	CodePipelineLoop               = "pipeline_loop"
	CodePipelineHopsExceeded       = "pipeline_hops_exceeded"
	CodeHandlerError               = "handler_error"
	CodeServerClosed               = "server_closed"
	CodeBadPayload                 = "bad_payload"
	CodeBadArguments               = "bad_arguments"
	CodeFrameTooLarge              = "frame_too_large"
	CodeCapabilityConflict         = "capability_conflict"
	CodeUINotMounted               = "ui_not_mounted"
	CodeUIOwnerMismatch            = "ui_owner_mismatch"
	CodeInstanceNotFound           = "instance_not_found"
	CodeInstanceExists             = "instance_exists"
	CodeInstanceClosed             = "instance_closed"
	CodeContextNotFound            = "context_not_found"
	CodeContextClosed              = "context_closed"
	CodeContextOwnerMismatch       = "context_owner_mismatch"
	CodeContextParticipantMismatch = "context_participant_mismatch"
	CodeContextBindingInvalid      = "context_binding_invalid"
)

// UI publish errors are returned by the Host WebUI publisher seam.
var (
	ErrUINotMounted    = errors.New(CodeUINotMounted)
	ErrUIOwnerMismatch = errors.New(CodeUIOwnerMismatch)
)
