// Command kjak-feather-notify-macos is a kjak external feather plugin
// (protocol v1) that posts a macOS notification with terminal-notifier.
package main

import (
	"context"
	"os"

	"github.com/magicdrive/kjak-notify-macos/internal/plugin"
)

func main() {
	os.Exit(plugin.Main(context.Background(), os.Stdin, os.Stdout, os.Stderr))
}
