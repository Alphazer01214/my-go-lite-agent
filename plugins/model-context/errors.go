package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

// Error codes this plugin raises. Platform codes (bad_arguments, handler_error)
// and the shared tool result code come from pluginsdk and are not repeated here.
const (
	codeContextBudgetExceeded = "context_budget_exceeded"
	codeContextDeriveFailed   = "context_derive_failed"
	codeContextRecordFailed   = "context_record_failed"
	codeModelContextNotFound  = "model_context_not_found"
)

// Constructors keep the code and the message together at the raise site.
func errContextBudgetExceeded(msg string) error {
	return pluginsdk.ErrCode(codeContextBudgetExceeded, msg)
}
func errContextDeriveFailed(msg string) error {
	return pluginsdk.ErrCode(codeContextDeriveFailed, msg)
}
func errContextRecordFailed(msg string) error {
	return pluginsdk.ErrCode(codeContextRecordFailed, msg)
}
func errModelContextNotFound(msg string) error {
	return pluginsdk.ErrCode(codeModelContextNotFound, msg)
}

// Platform codes keep their own constructors so the raise site stays short.
func errBadArguments(msg string) error {
	return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, msg)
}

func errHandler(msg string) error {
	return pluginsdk.ErrCode(pluginsdk.CodeHandlerError, msg)
}

func errCoded(code, msg string) error {
	return pluginsdk.ErrCode(code, msg)
}
