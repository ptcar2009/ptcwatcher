package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/radovskyb/watcher"
	"github.com/spf13/cobra"
)

func GlobFilter(globs []string) watcher.FilterFileHookFunc {
	return func(info os.FileInfo, fullPath string) error {
		curPath, err := os.Getwd()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(curPath, fullPath)
		if err != nil {
			return err
		}
		for _, glob := range globs {
			matched, err := filepath.Match(glob, rel)
			if err != nil {
				return watcher.ErrSkip
			}

			if matched {
				return nil
			}
		}

		return watcher.ErrSkip
	}
}
func IgnoreFilter(globs []string) watcher.FilterFileHookFunc {
	return func(info os.FileInfo, fullPath string) error {
		curPath, err := os.Getwd()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(curPath, fullPath)
		if err != nil {
			return err
		}
		for _, glob := range globs {
			matched, err := filepath.Match(glob, rel)
			if err != nil {
				return watcher.ErrSkip
			}

			if matched {
				return watcher.ErrSkip
			}
		}

		return nil
	}
}

var mainCommand *cobra.Command = &cobra.Command{
	Use:   "ptcwatcher <commands> [flags]",
	Short: "file watcher for triggering commands on file changes",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, _ := cmd.Flags().GetStringSlice("watch")
		w := watcher.New()

		// SetMaxEvents to 1 to allow at most 1 event's to be received
		// on the Event channel per watching cycle.
		//
		// If SetMaxEvents is not set, the default is to send all events.
		w.SetMaxEvents(1)
		w.FilterOps(watcher.Rename, watcher.Create, watcher.Write)

		ignore, err := cmd.Flags().GetString("ignore-file")
		if ignore != "" && err != nil {
			return err
		}
		if ignore != "" {

			f, err := os.ReadFile(ignore)
			if err != nil {
				return err
			}

			ignores := strings.Split(string(f), "\n")
			w.AddFilterHook(IgnoreFilter(ignores))
		}
		ignores, _ := cmd.Flags().GetStringSlice("ignore")
		w.AddFilterHook(IgnoreFilter(ignores))

		for _, f := range wd {
			// Watch this folder for changes.
			if err := w.AddRecursive(f); err != nil {
				log.Fatalln(err)
			}
		}

		go func(ctx context.Context) {
			for {
				select {
				case <-w.Event:
					done := make(chan struct{})
					go func() {
						hasEvent := false
					selectFor:
						for {
							select {
							case <-w.Event:
								hasEvent = true
							case <-done:
								break selectFor
							case <-ctx.Done():
								return
							}
						}
						if hasEvent {
							w.Event <- watcher.Event{}
						}
					}()
					commands := []*exec.Cmd{}
					for _, command := range args {
						all := strings.Split(command, " ")
						cmd := exec.CommandContext(ctx, all[0], all[1:]...)
						commands = append(commands, cmd)
					}
					//
					for _, command := range commands {
						out, err := command.CombinedOutput()
						fmt.Print(string(out))
						if err != nil {
							break
						}
					}
					done <- struct{}{}
				}
			}
		}(cmd.Context())

		w.Start(1000 * time.Millisecond)
		return nil
	},
}

func main() {
	mainCommand.Flags().StringSliceP("watch", "w", []string{"."}, "watch the file or directory")
	mainCommand.Flags().StringP("ignore-file", "I", "", "ignore glob patterns in referred file")
	mainCommand.Flags().StringSliceP("ignore", "i", []string{".git/*"}, "ignore glob patterns")
	mainCommand.Execute()
}
