//go:build !linux && !windows

package desktop

import (
	"log"
	"os/exec"
)

func EnsureElevated() (relaunched bool) { return false }
func Fatal(format string, a ...any)     { log.Fatalf(format, a...) }
func (s *Shell) Show()                  { _ = exec.Command("open", s.url).Start() }
func (s *Shell) Close()                 {}
func RunWindow(string, string) int      { return 2 }

func AllowFocus() {}
