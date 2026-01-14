// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package logutil

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"

	commonconstants "github.com/gardener/scaling-advisor/api/common/constants"
	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
)

// VerbosityFromContext retrieves the verbosity level from the given context.
func VerbosityFromContext(ctx context.Context) int {
	v := ctx.Value(commonconstants.VerbosityCtxKey)
	if v == nil {
		return 0
	}
	verbosity, ok := v.(int)
	if !ok {
		return 0
	}
	return verbosity
}

// WrapContextWithFileLogger wraps the logr logger obtained from the given context with a multi-sink logr logger that
// logs to the original sink as well as a sink to the given filePath.
// It returns a new context containing this new multi-sink logr logger, a closer for the log file at path or any error encountered during setup.
func WrapContextWithFileLogger(ctx context.Context, prefix string, path string) (logCtx context.Context, closer io.Closer, err error) {
	logFile, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	closer = logFile
	fileLogger := stdr.New(log.New(logFile, prefix, log.LstdFlags))
	fileSink := fileLogger.GetSink()

	base := logr.FromContextOrDiscard(ctx) // get the base logger from the context
	mSink := &multiSink{sinks: []logr.LogSink{base.GetSink(), fileSink}}

	combined := logr.New(mSink).WithCallDepth(1)
	logCtx = context.WithValue(logr.NewContext(ctx, combined), commonconstants.TraceLogPathCtxKey, path)

	return
}

// multiSink forwards to multiple sinks (e.g., original + file).
type multiSink struct {
	sinks []logr.LogSink
}

var _ logr.LogSink = (*multiSink)(nil)

func (m *multiSink) Init(info logr.RuntimeInfo) {
	for _, s := range m.sinks {
		s.Init(info)
	}
}

func (m *multiSink) Enabled(level int) bool {
	for _, s := range m.sinks {
		if s.Enabled(level) {
			return true
		}
	}
	return false
}

func (m *multiSink) Info(level int, msg string, kvs ...interface{}) {
	for _, s := range m.sinks {
		s.Info(level, msg, kvs...)
	}
}

func (m *multiSink) Error(err error, msg string, kvs ...interface{}) {
	for _, s := range m.sinks {
		s.Error(err, msg, kvs...)
	}
}

func (m *multiSink) WithName(name string) logr.LogSink {
	newSinks := make([]logr.LogSink, len(m.sinks))
	for i, s := range m.sinks {
		newSinks[i] = s.WithName(name)
	}
	return &multiSink{sinks: newSinks}
}

func (m *multiSink) WithValues(keyValues ...any) logr.LogSink {
	newSinks := make([]logr.LogSink, len(m.sinks))
	for i, s := range m.sinks {
		newSinks[i] = s.WithValues(keyValues...)
	}
	return &multiSink{sinks: newSinks}
}
