package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

// Error codes this plugin raises. Platform codes (bad_arguments, handler_error)
// and the shared tool result code come from pluginsdk and are not repeated here.
const (
	codeEditAmbiguous     = "edit_ambiguous"
	codeEditNotFound      = "edit_not_found"
	codeFileBinary        = "file_binary"
	codeFileConflict      = "file_conflict"
	codeFileExists        = "file_exists"
	codeFileIo            = "file_io"
	codeFileTooLarge      = "file_too_large"
	codeLineRangeInvalid  = "line_range_invalid"
	codePathEscape        = "path_escape"
	codePathInvalid       = "path_invalid"
	codePathIsDirectory   = "path_is_directory"
	codePathNotDirectory  = "path_not_directory"
	codePathNotFound      = "path_not_found"
	codeWorkspaceInvalid  = "workspace_invalid"
	codeWorkspaceRequired = "workspace_required"
)

// Constructors keep the code and the message together at the raise site.
func errEditAmbiguous(msg string) error     { return pluginsdk.ErrCode(codeEditAmbiguous, msg) }
func errEditNotFound(msg string) error      { return pluginsdk.ErrCode(codeEditNotFound, msg) }
func errFileBinary(msg string) error        { return pluginsdk.ErrCode(codeFileBinary, msg) }
func errFileConflict(msg string) error      { return pluginsdk.ErrCode(codeFileConflict, msg) }
func errFileExists(msg string) error        { return pluginsdk.ErrCode(codeFileExists, msg) }
func errFileIo(msg string) error            { return pluginsdk.ErrCode(codeFileIo, msg) }
func errFileTooLarge(msg string) error      { return pluginsdk.ErrCode(codeFileTooLarge, msg) }
func errLineRangeInvalid(msg string) error  { return pluginsdk.ErrCode(codeLineRangeInvalid, msg) }
func errPathEscape(msg string) error        { return pluginsdk.ErrCode(codePathEscape, msg) }
func errPathInvalid(msg string) error       { return pluginsdk.ErrCode(codePathInvalid, msg) }
func errPathIsDirectory(msg string) error   { return pluginsdk.ErrCode(codePathIsDirectory, msg) }
func errPathNotDirectory(msg string) error  { return pluginsdk.ErrCode(codePathNotDirectory, msg) }
func errPathNotFound(msg string) error      { return pluginsdk.ErrCode(codePathNotFound, msg) }
func errWorkspaceInvalid(msg string) error  { return pluginsdk.ErrCode(codeWorkspaceInvalid, msg) }
func errWorkspaceRequired(msg string) error { return pluginsdk.ErrCode(codeWorkspaceRequired, msg) }
