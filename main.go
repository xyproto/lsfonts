// lsfonts lists every font installed on the system as a scrollable,
// searchable, visually-rendered list. When the terminal supports an
// inline image protocol (Kitty, iTerm2, or Sixel) every entry is shown
// rendered in its own typeface; otherwise lsfonts prints a plain text
// listing of the discovered fonts.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/xyproto/imagepreview"
)

// version is printed by -v/--version. Bumped manually with each release.
const version = "0.1.0"

// usage is the text shown for -h/--help and on flag errors. Keeping the
// message inline (instead of relying on flag.PrintDefaults) lets us match
// the wording of the man page so users see the same help in both places.
const usage = `lsfonts ` + version + ` -- list installed fonts with rendered previews

Usage:
  lsfonts [options]

Options:
  -l                   print plain text list and exit
  -v, --version        print version and exit
  -h, --help           show this help and exit

Interactive keys:
  Up/Down, k/j         move selection (work inside search too)
  PgUp/PgDn, Space, b  page up / page down
  g, Home              jump to top
  G, End               jump to bottom
  /                    open the filter prompt (preserves the current query)
  Enter                keep the filter, leave the prompt
  Backspace, Ctrl-U    edit / clear the filter while typing
  Esc                  clear an active filter, or quit when none is active
  q, Ctrl-C            quit

When stdout is not a Kitty, iTerm2, or Sixel terminal, lsfonts falls back
to the plain text listing automatically.
`

func main() {
	// Register flags. The standard flag package treats "-name" and
	// "--name" identically, so registering "version" gives both -version
	// and --version. The "-v" short form is a separate bool flag pointing
	// at the same destination so both forms can be passed.
	var (
		listOnly    bool
		showVersion bool
	)
	fs := flag.NewFlagSet("lsfonts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&listOnly, "l", false, "print plain text list and exit")
	fs.BoolVar(&showVersion, "v", false, "print version and exit")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	if err := fs.Parse(os.Args[1:]); err != nil {
		// flag.ContinueOnError already wrote a diagnostic to Stderr.
		// -h / --help is reported as flag.ErrHelp; treat it as success.
		if err == flag.ErrHelp {
			fmt.Fprint(os.Stdout, usage)
			return
		}
		os.Exit(2)
	}

	if showVersion {
		fmt.Println("lsfonts", version)
		return
	}

	entries := DiscoverFonts()
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "lsfonts: no fonts found in the standard system directories")
		os.Exit(1)
	}

	if listOnly || !imagepreview.HasGraphics {
		printPlainList(entries)
		return
	}

	if err := runUI(entries); err != nil {
		fmt.Fprintln(os.Stderr, "lsfonts:", err)
		os.Exit(1)
	}
}

// printPlainList writes one line per entry as "Family — Style\tpath[:index]"
// to stdout. Used as the fallback for terminals without inline graphics and
// when the user passes -l.
func printPlainList(entries []FontEntry) {
	for _, e := range entries {
		if e.Index > 0 {
			fmt.Printf("%s\t%s:%d\n", e.Display, e.Path, e.Index)
		} else {
			fmt.Printf("%s\t%s\n", e.Display, e.Path)
		}
	}
}
