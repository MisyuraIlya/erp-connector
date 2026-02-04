package main

import (
	"bytes"
	"io"
	"sync"
)

type logWatcher struct {
	dst   io.Writer
	once  sync.Once
	onHit func()
}

func newLogWatcher(dst io.Writer, onHit func()) io.Writer {
	if dst == nil {
		dst = io.Discard
	}
	return &logWatcher{dst: dst, onHit: onHit}
}

func (w *logWatcher) Write(p []byte) (int, error) {
	if w.onHit != nil && isOpenGLFailureLog(p) {
		w.once.Do(func() {
			go w.onHit()
		})
	}
	return w.dst.Write(p)
}

func isOpenGLFailureLog(p []byte) bool {
	if len(p) == 0 {
		return false
	}
	return bytes.Contains(p, []byte("WGL: The driver does not appear to support OpenGL")) ||
		bytes.Contains(p, []byte("APIUnavailable: WGL")) ||
		bytes.Contains(p, []byte("window creation error"))
}
