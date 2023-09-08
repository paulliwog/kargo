package delete

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
	pkgerrors "github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"

	"github.com/akuity/kargo/internal/cli/client"
	"github.com/akuity/kargo/internal/cli/option"
	v1alpha1 "github.com/akuity/kargo/pkg/api/service/v1alpha1"
)

func newStageCommand(opt *option.Option) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stage [NAME]...",
		Short: "Delete stage by name",
		Args:  cobra.MinimumNArgs(1),
		Example: `
# Delete stage
kargo delete stage --project=my-project my-stage
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			kargoSvcCli, err := client.GetClientFromConfig(ctx, opt)
			if err != nil {
				return pkgerrors.New("get client from config")
			}

			project := opt.Project.OrElse("")
			if project == "" {
				return errors.New("project is required")
			}

			var resErr error
			for _, name := range slices.Compact(args) {
				if _, err := kargoSvcCli.DeleteStage(ctx, connect.NewRequest(&v1alpha1.DeleteStageRequest{
					Project: project,
					Name:    name,
				})); err != nil {
					resErr = errors.Join(resErr, pkgerrors.Wrap(err, "Error"))
					continue
				}
				_, _ = fmt.Fprintf(opt.IOStreams.Out, "Stage Deleted: %q\n", name)
			}
			return resErr
		},
	}
	opt.PrintFlags.AddFlags(cmd)
	option.OptionalProject(opt.Project)(cmd.Flags())
	return cmd
}
