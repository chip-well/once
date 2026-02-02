package command

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basecamp/amar/internal/docker"
	"github.com/basecamp/amar/internal/ui"
)

type RootCommand struct {
	cmd       *cobra.Command
	namespace string
}

func NewRootCommand() *RootCommand {
	r := &RootCommand{}
	r.cmd = &cobra.Command{
		Use:   "amar",
		Short: "Manage web applications from Docker images",
		RunE: WithNamespace(func(ns *docker.Namespace, cmd *cobra.Command, args []string) error {
			return ui.Run(ns)
		}),
	}
	r.cmd.PersistentFlags().StringVarP(&r.namespace, "namespace", "n", docker.DefaultNamespace, "namespace for containers")

	r.cmd.AddCommand(NewBackupCommand(r).Command())
	r.cmd.AddCommand(NewDeployCommand(r).Command())
	r.cmd.AddCommand(NewListCommand(r).Command())
	r.cmd.AddCommand(NewRestoreCommand(r).Command())
	r.cmd.AddCommand(NewStartCommand(r).Command())
	r.cmd.AddCommand(NewStopCommand(r).Command())
	r.cmd.AddCommand(NewTeardownCommand(r).Command())

	return r
}

func (r *RootCommand) Execute() error {
	return r.cmd.Execute()
}

// Helpers

type NamespaceRunE func(ns *docker.Namespace, cmd *cobra.Command, args []string) error

func WithNamespace(fn NamespaceRunE) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		namespace, _ := cmd.Root().PersistentFlags().GetString("namespace")

		ns, err := docker.RestoreNamespace(ctx, namespace)
		if err != nil {
			return fmt.Errorf("restoring namespace: %w", err)
		}

		return fn(ns, cmd, args)
	}
}
