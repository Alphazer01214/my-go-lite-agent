package logging

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	defaultMaxBytes = 8 << 20
	defaultBackups  = 3
)

// Writer appends to one file and rotates it by size.
type Writer struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	backups  int
	file     *os.File
	size     int64
}

func New(path string, maxBytes int64, backups int) (*Writer, error) {
	if path == "" {
		return nil, errors.New("log path is required")
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	if backups < 0 {
		backups = 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	w := &Writer{path: path, maxBytes: maxBytes, backups: backups}
	if err := w.open(); err != nil {
		return nil, err
	}
	if w.size >= w.maxBytes {
		w.mu.Lock()
		err := w.rotateLocked()
		w.mu.Unlock()
		if err != nil {
			return nil, err
		}
	}
	return w, nil
}

func (w *Writer) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *Writer) rotateLocked() error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if w.backups == 0 {
		if err := os.Remove(w.path); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else {
		for i := w.backups - 1; i >= 1; i-- {
			old := fmt.Sprintf("%s.%d", w.path, i)
			next := fmt.Sprintf("%s.%d", w.path, i+1)
			_ = os.Remove(next)
			_ = os.Rename(old, next)
		}
		_ = os.Remove(w.path + ".1")
		if err := os.Rename(w.path, w.path+".1"); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return w.open()
}
