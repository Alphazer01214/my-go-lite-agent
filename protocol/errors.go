package protocol

// Stable wire error_code values (protocol.md §7).
// Written to Frame.ErrorCode; SDK uses Code(err) / ErrCode(code, msg) on top.
const (
	CodeHostClosed             = "host_closed"
	CodeRouteFailed            = "route_failed"
	CodeMethodNotFound         = "method_not_found"
	CodePluginNotMounted       = "plugin_not_mounted"
	CodePluginDown             = "plugin_down"
	CodePluginDisabled         = "plugin_disabled"
	CodeTimeout                = "timeout"
	CodeHandlerError           = "handler_error"
	CodeHostServerClosed       = "server_closed"
	CodeServerClosed           = "server_closed"
	CodeBadPayload             = "bad_payload"
	CodeBadArguments           = "bad_arguments"
	CodeFrameTooLarge          = "frame_too_large"
	CodeEnsurePluginsFailed    = "ensure_plugins_failed"
	CodeSetPluginEnabledFailed = "set_plugin_enabled_failed"
	CodeCapabilityConflict     = "capability_conflict"
	CodeHostFaceNotDeclared    = "host_face_not_declared"
	CodePanelRejected          = "panel_rejected"
)
