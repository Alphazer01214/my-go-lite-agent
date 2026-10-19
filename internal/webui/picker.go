package webui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var errDirectoryPickerUnsupported = errors.New("directory picker is not supported on this platform")

type directoryPicker func(context.Context) (path string, cancelled bool, err error)

type pickDirectoryOut struct {
	Path      string `json:"path,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

const macOSPickDirectoryScript = `try
    POSIX path of (choose folder with prompt "选择工作区文件夹")
on error number -128
    return ""
end try`

// The Windows dialog output is base64 so the path crosses the PowerShell
// console encoding unchanged, including non-ASCII directory names.
const windowsPickDirectoryScript = `Add-Type -AssemblyName System.Windows.Forms
$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
$dialog.Description = '选择工作区文件夹'
if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
    [Console]::Out.Write([Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($dialog.SelectedPath)))
}`

// servePickDirectory exposes the Host-side folder chooser. Browsers do not
// expose an absolute path for a directory selection; Workspace registration
// needs one.
func (s *Server) servePickDirectory(w http.ResponseWriter, r *http.Request) {
	picker := s.pickDirectory
	if picker == nil {
		picker = pickDirectoryNative
	}
	path, cancelled, err := picker(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			// The browser went away while the dialog was open.
			return
		case errors.Is(err, errDirectoryPickerUnsupported):
			writeManagementError(w, http.StatusNotImplemented, "folder_picker_unsupported", err.Error())
		default:
			writeManagementError(w, http.StatusInternalServerError, "folder_picker_failed", err.Error())
		}
		return
	}
	if cancelled {
		writePickDirectory(w, pickDirectoryOut{Cancelled: true})
		return
	}
	if strings.TrimSpace(path) == "" {
		writeManagementError(w, http.StatusInternalServerError, "folder_picker_failed", "folder picker returned an empty path")
		return
	}
	writePickDirectory(w, pickDirectoryOut{Path: path})
}

func writePickDirectory(w http.ResponseWriter, out pickDirectoryOut) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}

// pickDirectoryNative opens the platform folder chooser and returns an
// absolute path, or cancelled when the user dismisses it.
func pickDirectoryNative(ctx context.Context) (string, bool, error) {
	switch runtime.GOOS {
	case "darwin":
		return pickDirectoryMacOS(ctx)
	case "windows":
		return pickDirectoryWindows(ctx)
	case "linux":
		return pickDirectoryLinux(ctx)
	default:
		return "", false, errDirectoryPickerUnsupported
	}
}

func pickDirectoryMacOS(ctx context.Context) (string, bool, error) {
	output, err := runPickerCommand(ctx, "osascript", "-e", macOSPickDirectoryScript)
	if err != nil {
		return "", false, err
	}
	return pickerResult(output)
}

func pickDirectoryWindows(ctx context.Context) (string, bool, error) {
	output, err := runPickerCommand(ctx, "powershell.exe", "-NoProfile", "-STA", "-WindowStyle", "Hidden", "-Command", windowsPickDirectoryScript)
	if err != nil {
		return "", false, err
	}
	return decodeWindowsPickerOutput(output)
}

func decodeWindowsPickerOutput(output string) (string, bool, error) {
	encoded := strings.TrimSpace(output)
	if encoded == "" {
		return "", true, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", false, fmt.Errorf("decode folder picker output: %w", err)
	}
	return pickerResult(string(decoded))
}

func pickDirectoryLinux(ctx context.Context) (string, bool, error) {
	output, err := runPickerCommand(ctx, "zenity", "--file-selection", "--directory", "--title", "选择工作区文件夹")
	if err == nil {
		return pickerResult(output)
	}
	if linuxPickerCancelled(err) {
		return "", true, nil
	}
	if !errors.Is(err, exec.ErrNotFound) {
		return "", false, err
	}

	startDir, _ := os.UserHomeDir()
	if startDir == "" {
		startDir = "."
	}
	output, err = runPickerCommand(ctx, "kdialog", "--title", "选择工作区文件夹", "--getexistingdirectory", startDir)
	if err == nil {
		return pickerResult(output)
	}
	if linuxPickerCancelled(err) {
		return "", true, nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "", false, errDirectoryPickerUnsupported
	}
	return "", false, err
}

func runPickerCommand(ctx context.Context, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.Output()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", &pickerCommandError{name: name, err: err, stderr: pickerStderr(err)}
	}
	return string(output), nil
}

func pickerResult(output string) (string, bool, error) {
	path := strings.TrimRight(output, "\r\n")
	if path == "" {
		return "", true, nil
	}
	if !filepath.IsAbs(path) {
		return "", false, errors.New("folder picker returned a relative path")
	}
	return filepath.Clean(path), false, nil
}

func linuxPickerCancelled(err error) bool {
	var commandErr *pickerCommandError
	return errors.As(err, &commandErr) && commandErr.exitCode() == 1
}

func pickerStderr(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return strings.TrimSpace(string(exitErr.Stderr))
	}
	return ""
}

type pickerCommandError struct {
	name   string
	err    error
	stderr string
}

func (e *pickerCommandError) Error() string {
	message := fmt.Sprintf("folder picker %s failed: %v", e.name, e.err)
	if e.stderr != "" {
		message += ": " + e.stderr
	}
	return message
}

func (e *pickerCommandError) Unwrap() error {
	return e.err
}

func (e *pickerCommandError) exitCode() int {
	var exitErr *exec.ExitError
	if errors.As(e.err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
