package commands

import (
	"context"

	"github.com/fatih/color"
	"github.com/gosuri/uitable"
	"github.com/oam-dev/kubevela/api/types"
	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	"github.com/oam-dev/kubevela/pkg/plugins"
	"github.com/oam-dev/kubevela/pkg/utils/system"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type refreshStatus string

const (
	added     refreshStatus = "Added"
	updated   refreshStatus = "Updated"
	unchanged refreshStatus = "Unchanged"
	deleted   refreshStatus = "Deleted"
)

func NewRefreshCommand(c types.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "update",
		DisableFlagsInUseLine: true,
		Short:                 "Sync definition from cluster",
		Long:                  "Refresh and sync definition files from cluster",
		Example:               `vela system update`,
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := client.New(c.Config, client.Options{Scheme: c.Schema})
			if err != nil {
				return err
			}
			return RefreshDefinitions(ctx, newClient, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.SetOut(ioStreams.Out)
	return cmd
}

func RefreshDefinitions(ctx context.Context, c client.Client, ioStreams cmdutil.IOStreams) error {
	ioStreams.Infof("Synchronizing capabilities from cluster%s...\n", emojiWait)
	dir, _ := system.GetCapabilityDir()

	oldCaps, err := plugins.LoadAllInstalledCapability()
	if err != nil {
		return err
	}
	syncedTemplates := []types.Capability{}
	//TODO show errors of handling templates
	templates, _, err := plugins.GetWorkloadsFromCluster(ctx, types.DefaultOAMNS, c, dir, nil)
	if err != nil {
		return err
	}
	syncedTemplates = append(syncedTemplates, templates...)
	plugins.SinkTemp2Local(templates, dir)

	//TODO show errors of handling templates
	templates, _, err = plugins.GetTraitsFromCluster(ctx, types.DefaultOAMNS, c, dir, nil)
	if err != nil {
		return err
	}
	syncedTemplates = append(syncedTemplates, templates...)
	plugins.SinkTemp2Local(templates, dir)
	plugins.RemoveLegacyTemps(syncedTemplates, dir)

	printRefreshReport(syncedTemplates, oldCaps, ioStreams)
	return nil
}

func printRefreshReport(newCaps, oldCaps []types.Capability, io cmdutil.IOStreams) {
	report := refreshResultReport(newCaps, oldCaps)
	table := uitable.New()
	table.AddRow("TYPE", "CATEGORY", "DESCRIPTION")

	if len(report[added]) == 0 && len(report[updated]) == 0 && len(report[deleted]) == 0 {
		// no change occurs, just show all existing caps
		// always show workload at first
		for _, cap := range report[unchanged] {
			if cap.Type == types.TypeWorkload {
				table.AddRow(cap.Name, cap.Type, cap.Description)
			}
		}
		for _, cap := range report[unchanged] {
			if cap.Type == types.TypeTrait {
				table.AddRow(cap.Name, cap.Type, cap.Description)
			}
		}
		io.Infof("Sync capabilities successfully %s(no changes)\n", emojiSucceed)
		io.Info(table.String())
		return
	}

	io.Infof("Sync capabilities successfully %sAdd(%s) Update(%s) Delete(%s)\n",
		emojiSucceed,
		green.Sprint(len(report[added])),
		yellow.Sprint(len(report[updated])),
		red.Sprint(len(report[deleted])))
	// show added/updated/deleted cpas
	addStsRow(added, report, table)
	addStsRow(updated, report, table)
	addStsRow(deleted, report, table)
	io.Info(table.String())
}

func addStsRow(sts refreshStatus, report map[refreshStatus][]types.Capability, t *uitable.Table) {
	caps := report[sts]
	if len(caps) == 0 {
		return
	}
	var stsIcon string
	var stsColor *color.Color
	switch sts {
	case added:
		stsIcon = "+"
		stsColor = green
	case updated:
		stsIcon = "*"
		stsColor = yellow
	case deleted:
		stsIcon = "-"
		stsColor = red
	}
	for _, cap := range caps {
		t.AddRow(
			// color.New(color.Bold).Sprint(stsColor.Sprint(stsIcon)),
			stsColor.Sprintf("%s%s", stsIcon, cap.Name),
			stsColor.Sprint(cap.Type),
			stsColor.Sprint(cap.Description))
	}
}

func refreshResultReport(newCaps, oldCaps []types.Capability) map[refreshStatus][]types.Capability {
	report := map[refreshStatus][]types.Capability{
		added:     make([]types.Capability, 0),
		updated:   make([]types.Capability, 0),
		unchanged: make([]types.Capability, 0),
		deleted:   make([]types.Capability, 0),
	}
	for _, newCap := range newCaps {
		found := false
		for _, oldCap := range oldCaps {
			if newCap.Name == oldCap.Name {
				found = true
				break
			}
		}
		if !found {
			report[added] = append(report[added], newCap)
		}
	}
	for _, oldCap := range oldCaps {
		found := false
		for _, newCap := range newCaps {
			if oldCap.Name == newCap.Name {
				found = true
				if types.EqualCapability(oldCap, newCap) {
					report[unchanged] = append(report[unchanged], newCap)
				} else {
					report[updated] = append(report[updated], newCap)
				}
				break
			}
		}
		if !found {
			report[deleted] = append(report[deleted], oldCap)
		}
	}
	return report
}
