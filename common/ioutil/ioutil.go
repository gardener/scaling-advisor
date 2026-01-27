// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ioutil

import (
	"embed"
	"fmt"
	"io"
	"os"
	"text/template"

	commonerrors "github.com/gardener/scaling-advisor/api/common/errors"
	"k8s.io/apimachinery/pkg/api/resource"
)

// CloserFunc is a function type that implements io.Closer.
type CloserFunc func() error

// Close releases resources associated with the CloserFunc implementation by invoking the function it wraps.
func (f CloserFunc) Close() error {
	return f()
}

// CloseQuietly safely closes an io.Closer, ignoring and suppressing any error during the close operation.
func CloseQuietly(closer io.Closer) {
	if closer != nil {
		_ = closer.Close()
	}
}

// LoadEmbeddedTextTemplate loads the text template at the given templatePath within the given embedFS.
// If unsuccessful, returns an error containing the sentinel error commonerrors.ErrLoadTemplate.
func LoadEmbeddedTextTemplate(embedFS embed.FS, templatePath string) (tmpl *template.Template, err error) {
	data, err := embedFS.ReadFile(templatePath)
	if err != nil {
		err = fmt.Errorf("%w: cannot read %q from embed FS: %w", commonerrors.ErrLoadTemplate, templatePath, err)
		return
	}
	tmpl, err = template.New(templatePath).Funcs(funcMap).Parse(string(data))
	if err != nil {
		err = fmt.Errorf("%w: cannot parse %q template: %w", commonerrors.ErrLoadTemplate, templatePath, err)
	}
	return
}

// GetTempDir gets the temp directory for trace logs, generated files, etc preferring `/tmp` if present.
func GetTempDir() string {
	if slashTmpDirExists {
		return "/tmp"
	} else {
		return os.TempDir()
	}
}

func init() {
	info, err := os.Stat("/tmp")
	slashTmpDirExists = (err == nil) && info.IsDir()
}

var (
	slashTmpDirExists bool
	funcMap           = template.FuncMap{
		"toString": func(v any) string {
			if q, ok := v.(resource.Quantity); ok {
				return q.String()
			}
			if p, ok := v.(*resource.Quantity); ok {
				return p.String()
			}
			// Generic fallback
			if s, ok := v.(fmt.Stringer); ok {
				return s.String()
			}
			return fmt.Sprintf("%v", v)
		},
	}
)
