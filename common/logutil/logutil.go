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
	"regexp"

	"github.com/gardener/scaling-advisor/common/objutil"

	commontypes "github.com/gardener/scaling-advisor/api/common/types"
	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DefaultDumpVerbosity represents the verbosity level at which objects are dumped into the file system for diagnosis
const (
	DefaultDumpVerbosity = 5
)

// VerbosityFromContext retrieves the verbosity level from the given context.
func VerbosityFromContext(ctx context.Context) (verbosity uint32) {
	val := ctx.Value(commontypes.VerbosityCtxKey)
	if val == nil {
		return
	}
	var ok bool
	verbosity, ok = val.(uint32)
	if ok {
		return
	}
	v, ok := val.(int) // check if int
	if ok && v >= 0 {
		verbosity = uint32(v) // #nosec G115 -- safe: v >= 0 already checked
	}
	return
}

// VerbosityTraceFromContext retrieves the verbosity level, the trace dir and the trace log path from the given context.
func VerbosityTraceFromContext(ctx context.Context) (verbosity uint32, traceDir string, traceLogPath string) {
	verbosity = VerbosityFromContext(ctx)
	d := ctx.Value(commontypes.TraceDirCtxKey)
	if d != nil {
		traceDir = d.(string)
	}
	l := ctx.Value(commontypes.TraceLogPathCtxKey)
	if l != nil {
		traceLogPath = l.(string)
	}
	return
}

// DumpObjectIfNeeded dumps the YAML for the given object into a `object-name.yaml` file within a `traceDir` obtained
// from the context if any
func DumpObjectIfNeeded(ctx context.Context, obj metav1.Object) error {
	l := logr.FromContextOrDiscard(ctx)
	verbosity, traceDir, _ := VerbosityTraceFromContext(ctx)
	if verbosity < DefaultDumpVerbosity || traceDir == "" {
		return nil
	}
	runtimeObj := obj.(runtime.Object)
	var yamlDumpPath string
	yamlDumpPath, err := objutil.SaveRuntimeObjAsYAMLToPath(runtimeObj, traceDir, obj.GetName()+".yaml")
	if err != nil {
		return err
	}
	l.V(DefaultDumpVerbosity).Info("dumped object", "kind", runtimeObj.GetObjectKind().GroupVersionKind().Kind,
		"yamlDumpPath", yamlDumpPath, "name", obj.GetName(), "namespace", obj.GetNamespace())
	return nil
}

// GetCleanLogFileName removes all special characters from fileName and returns the clean fileName
func GetCleanLogFileName(fileName string) string {
	return fileNameCleanRe.ReplaceAllString(fileName, "")
}

// WrapContextWithFileLogger wraps the logr logger obtained from the given context with a multi-sink logr logger that
// logs to the original sink as well as a sink to the given filePath.
// It returns a new context containing this new multi-sink logr logger, a closer for the log file at path or any error encountered during setup.
func WrapContextWithFileLogger(ctx context.Context, prefix string, logPath string) (logCtx context.Context, closer io.Closer, err error) {
	logFile, err := os.Create(filepath.Clean(logPath))
	if err != nil {
		return
	}
	closer = logFile
	fileLogger := stdr.New(log.New(logFile, prefix, log.LstdFlags|log.Lshortfile))
	fileSink := fileLogger.GetSink()
	if cd, ok := fileSink.(logr.CallDepthLogSink); ok {
		fileSink = cd.WithCallDepth(1)
	}

	base := logr.FromContextOrDiscard(ctx) // get the base logger from the context
	baseSink := base.GetSink()
	if cd, ok := baseSink.(logr.CallDepthLogSink); ok {
		baseSink = cd.WithCallDepth(1)
	}
	mSink := &multiSink{sinks: []logr.LogSink{baseSink, fileSink}}

	combined := base.WithSink(mSink)
	logCtx = context.WithValue(logr.NewContext(ctx, combined), commontypes.TraceLogPathCtxKey, logPath)

	return
}

var (
	_ logr.LogSink          = (*multiSink)(nil)
	_ logr.CallDepthLogSink = (*multiSink)(nil) // If a sink implements CallDepthLogSink, logr will use it to adjust the call stack depth correctly.
)

func (m *multiSink) Init(info logr.RuntimeInfo) {
	for _, s := range m.sinks {
		s.Init(info)
	}
}

// multiSink forwards to multiple sinks (e.g., original + file).
type multiSink struct {
	sinks []logr.LogSink
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

// WithCallDepth returns a new multiSink that increases the call depth of all
// underlying sinks by the provided depth, plus one additional frame to account
// for the multiSink wrapper itself.
//
// Ensures that caller information (file and line number) reported by
// downstream sinks correctly reflects the original logging call site rather
// than the multiSink forwarding layer.
//
// If an underlying sink does not implement logr.CallDepthLogSink, it is
// returned unchanged.
func (m *multiSink) WithCallDepth(depth int) logr.LogSink {
	newSinks := make([]logr.LogSink, len(m.sinks))
	for i, s := range m.sinks {
		if cd, ok := s.(logr.CallDepthLogSink); ok {
			newSinks[i] = cd.WithCallDepth(depth + 1)
		} else {
			newSinks[i] = s
		}
	}
	return &multiSink{sinks: newSinks}
}

var fileNameCleanRe = regexp.MustCompile(`[^\w.-]`)
