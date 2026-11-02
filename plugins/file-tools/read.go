package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

type readArgs struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	MaxBytes  int    `json:"max_bytes,omitempty"`
}

func (p *plugin) read(ctx context.Context, req *providerRequest, args *readArgs) (toolResult, error) {
	if err := checkContext(ctx); err != nil {
		return toolResult{}, err
	}
	if args.StartLine < 0 || args.EndLine < 0 || args.MaxBytes < 0 {
		return toolResult{}, badArguments("line ranges and max_bytes must not be negative")
	}
	if args.StartLine == 0 {
		args.StartLine = 1
	}
	if args.EndLine > 0 && args.EndLine < args.StartLine {
		return toolResult{}, badArguments("end_line must be greater than or equal to start_line")
	}
	if args.MaxBytes == 0 {
		args.MaxBytes = defaultReadBytes
	}
	if args.MaxBytes > maxReadBytes {
		args.MaxBytes = maxReadBytes
	}

	workspace, err := resolveWorkspace(req.Workspace)
	if err != nil {
		return toolResult{}, err
	}
	path, err := resolvePath(workspace, args.Path)
	if err != nil {
		return toolResult{}, err
	}

	file, err := os.Open(path)
	if err != nil {
		return fileError("read", args.Path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fileError("read", args.Path, err)
	}
	if info.IsDir() {
		return failure("path_is_directory", "read: path is a directory: "+args.Path), nil
	}

	if binary, err := looksBinary(file); err != nil {
		return fileError("read", args.Path, err)
	} else if binary {
		return failure("file_binary", "read: file is not UTF-8 text: "+args.Path), nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fileError("read", args.Path, err)
	}

	reader := bufio.NewReaderSize(file, 64*1024)
	var builder strings.Builder
	lineNo := 0
	lineStart := 0
	lineEnd := 0
	lineCount := 0
	truncated := false

	for {
		if err := checkContext(ctx); err != nil {
			return toolResult{}, err
		}
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			lineNo++
			if lineNo >= args.StartLine {
				if args.EndLine > 0 && lineNo > args.EndLine {
					break
				}
				if strings.IndexByte(line, 0) >= 0 || !utf8.ValidString(line) {
					return failure("file_binary", "read: file is not UTF-8 text: "+args.Path), nil
				}
				if builder.Len()+len(line) > args.MaxBytes {
					truncated = true
					break
				}
				if lineStart == 0 {
					lineStart = lineNo
				}
				builder.WriteString(line)
				lineEnd = lineNo
				lineCount++
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return fileError("read", args.Path, readErr)
		}
	}

	meta := map[string]any{
		"path":       args.Path,
		"size":       info.Size(),
		"line_start": lineStart,
		"line_end":   lineEnd,
		"line_count": lineCount,
		"truncated":  truncated,
	}
	result := success(builder.String(), meta)
	result.Truncated = truncated
	return result, nil
}

func looksBinary(file *os.File) (bool, error) {
	buffer := make([]byte, 8192)
	n, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	prefix := buffer[:n]
	if strings.IndexByte(string(prefix), 0) >= 0 {
		return true, nil
	}
	if n < len(buffer) && !utf8.Valid(prefix) {
		return true, nil
	}
	return false, nil
}

var (
	errFileBinary      = errors.New("file is not UTF-8 text")
	errFileTooLarge    = errors.New("file exceeds size limit")
	errPathIsDirectory = errors.New("path is a directory")
)

func readTextFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errPathIsDirectory
	}
	if info.Size() > maxEditFileBytes {
		return nil, fmt.Errorf("%w: %d bytes", errFileTooLarge, info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.IndexByte(string(data), 0) >= 0 || !utf8.Valid(data) {
		return nil, errFileBinary
	}
	return data, nil
}
