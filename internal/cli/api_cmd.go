package cli

import (
	"fmt"

	"github.com/alex-mccollum/igw-cli/internal/igwerr"
)

func (c *CLI) runAPI(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(c.Err, "Usage: igw api <list|show|search|tags|stats|capability|sync|refresh> [flags]")
		return &igwerr.UsageError{Msg: "required api subcommand"}
	}

	switch args[0] {
	case "list":
		return c.runAPIList(args[1:])
	case "show":
		return c.runAPIShow(args[1:])
	case "search":
		return c.runAPISearch(args[1:])
	case "tags":
		return c.runAPITags(args[1:])
	case "stats":
		return c.runAPIStats(args[1:])
	case "capability":
		return c.runAPICapability(args[1:])
	case "sync":
		return c.runAPISync(args[1:])
	case "refresh":
		return c.runAPIRefresh(args[1:])
	default:
		return &igwerr.UsageError{Msg: fmt.Sprintf("unknown api subcommand %q", args[0])}
	}
}
