package cli

import (
	"io"
	"os"
)

func (c *CLI) isInteractiveOutput() bool {
	if c == nil || c.Out == nil {
		return false
	}
	if c.IsTerminal != nil {
		return c.IsTerminal(c.Out)
	}
	return isWriterTerminal(c.Out)
}

func isWriterTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
