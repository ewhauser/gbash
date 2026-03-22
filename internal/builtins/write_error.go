package builtins

import (
	"errors"
	"os"
	"runtime"
	"strings"
)

func shellWriteErrorDiagnostic(name string, err error) (string, bool) {
	if runtime.GOOS == "darwin" {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && pathErr != nil && pathErr.Path != "" && pathErr.Err != nil {
			text := pathErr.Err.Error()
			if text == "" {
				return "", false
			}
			return pathErr.Path + ": " + strings.ToUpper(text[:1]) + text[1:], true
		}
	}
	text := shellWriteErrorText(err)
	if text == "" || name == "" {
		return "", false
	}
	return name + ": write error: " + text, true
}

func shellWriteErrorText(err error) string {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && pathErr != nil && pathErr.Err != nil {
		return pathErr.Err.Error()
	}
	if err == nil {
		return ""
	}
	return err.Error()
}
