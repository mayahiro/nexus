package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mayahiro/nexus/internal/browsermgr"
)

func printBrowserResults(w io.Writer, result browsermgr.SetupResult) {
	for _, browser := range result.Browsers {
		status := "unchanged"
		if browser.Changed {
			status = "updated"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", browser.Name, browser.Version, status, browser.ExecutablePath)
	}
}

func printBrowserStatus(w io.Writer, status browsermgr.Status) {
	for _, browser := range status.Browsers {
		state := "not_installed"
		if browser.Installed {
			state = "installed"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", browser.Name, browser.Version, state, browser.ExecutablePath)
	}
}

func printEvalValue(w io.Writer, value interface{}) error {
	switch value := value.(type) {
	case nil:
		_, err := fmt.Fprintln(w, "null")
		return err
	case string:
		_, err := fmt.Fprintln(w, value)
		return err
	default:
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
}
