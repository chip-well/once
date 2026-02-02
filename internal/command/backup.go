package command

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/basecamp/amar/internal/docker"
)

type BackupCommand struct {
	root *RootCommand
	cmd  *cobra.Command
}

func NewBackupCommand(root *RootCommand) *BackupCommand {
	b := &BackupCommand{root: root}
	b.cmd = &cobra.Command{
		Use:   "backup <app> <filename>",
		Short: "Backup an application to a file",
		Args:  cobra.ExactArgs(2),
		RunE:  WithNamespace(b.run),
	}
	return b
}

func (b *BackupCommand) Command() *cobra.Command {
	return b.cmd
}

// Private

func (b *BackupCommand) run(ns *docker.Namespace, cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	appName := args[0]
	filename := args[1]

	app := ns.Application(appName)
	if app == nil {
		return fmt.Errorf("application %q not found", appName)
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating backup file: %w", err)
	}
	defer file.Close()

	if err := app.Backup(ctx, file); err != nil {
		return fmt.Errorf("backing up application: %w", err)
	}

	fmt.Printf("Backed up %s to %s\n", appName, filename)
	return nil
}
