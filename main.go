// lsfonts lists installed fonts as a scrollable, searchable card view in
// terminals with inline graphics (Kitty/iTerm2/Sixel), or as a plain text
// list elsewhere.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/xyproto/imagepreview"
)

const version = "0.1.0"

// usage mirrors the man page so -h and lsfonts(1) stay in sync.
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
  Ctrl-C               copy the selected font name to the clipboard
                       (suitable for CSS, Vim/Neovim guifont, Pango, etc.)
  Esc                  clear an active filter, or quit when none is active
  q, Ctrl-D            quit

When stdout is not a Kitty, iTerm2, or Sixel terminal, lsfonts falls back
to the plain text listing automatically.
`

func main() {
	// flag treats -name and --name identically, so registering "version"
	// covers both --version and -version; -v is a separate alias.
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
		if err == flag.ErrHelp { // -h/--help is success, not an error
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

// printPlainList writes "Family — Style\tpath[:index]" per entry to stdout.
func printPlainList(entries []FontEntry) {
	for _, e := range entries {
		if e.Index > 0 {
			fmt.Printf("%s\t%s:%d\n", e.Display, e.Path, e.Index)
		} else {
			fmt.Printf("%s\t%s\n", e.Display, e.Path)
		}
	}
}
