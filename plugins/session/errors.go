package main

// Error codes this plugin raises. Platform codes (bad_arguments, handler_error)
// come from the platform and are not repeated here.
const (
	codeWorkspaceInvalid  = "workspace_invalid"
	codeWorkspaceMismatch = "workspace_mismatch"
	codeWorkspaceNotFound = "workspace_not_found"
	codeWorkspaceRequired = "workspace_required"
	codeSessionNotFound   = "session_not_found"
)

// errCoded builds a wire-coded error (pluginsdk.ErrCode compatible shape).
// Defined here so store.go stays free of a pluginsdk import cycle in tests.
func errCoded(code, msg string) error {
	return coded{code: code, msg: msg}
}

type coded struct {
	code string
	msg  string
}

func (e coded) Error() string { return e.msg }

func errBadArguments(msg string) error {
	return errCoded("bad_arguments", msg)
}

func errNotFound(id string) error {
	return errCoded(codeSessionNotFound, "session not found: "+id)
}

func errHandler(msg string) error {
	return errCoded("handler_error", msg)
}
