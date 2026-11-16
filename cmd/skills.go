// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"convo-cli/internal/affordance"
	"convo-cli/internal/errs"
	"convo-cli/internal/meta"
	"convo-cli/internal/officialskills"
	"convo-cli/internal/product"
	"convo-cli/internal/selfupdate"
	"convo-cli/internal/skillsfs"
)

// officialSkillsFS is the build-embedded official Skill content. It reuses the
// same embed the self-update installer ships (skills.Content), so `skills read`
// serves bytes identical to the release the binary was built from. Overridable
// in tests.
var officialSkillsFS fs.FS = officialskills.Current()

func skillsReader() (*skillsfs.Reader, error) {
	if officialSkillsFS == nil {
		return nil, errs.New("byteplus-rtc.skills.unavailable", errs.TypeInternal,
			"skill content is not embedded in this build").
			WithHint("use an official `%s` build", meta.BinName)
	}
	return skillsfs.New(officialSkillsFS), nil
}

func newSkillsCmd() *cobra.Command {
	skillName := product.Current().OfficialSkillName
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Inspect or synchronize binary-embedded official Skill content",
		Long: "Inspect or synchronize the official Agent scene workflows (SKILL.md + references) embedded in " +
			"this binary at build time, so they stay in lockstep with the CLI version. " +
			"Listing and reading require no auth or network; synchronization uses npx. Non-text resources are not served.",
	}
	cmd.AddCommand(newSkillsListCmd(), newSkillsReadCmd(), newSkillsSyncCmd())
	affordance.Attach(cmd, affordance.Affordance{
		When:  []string{"An Agent needs the version-matched workflow for a scene", "Official Skills need first installation, repair, or refresh"},
		Avoid: []string{"Editing embedded Skills"},
		Examples: []string{
			meta.BinName + " skills list",
			meta.BinName + " skills read " + skillName,
			meta.BinName + " skills sync",
		},
	})
	return cmd
}

type skillsSyncResult struct {
	Skill           string `json:"skill,omitempty"`
	PreviousVersion string `json:"previous_version,omitempty"`
	Version         string `json:"version"`
	Status          string `json:"status"`
	Action          string `json:"action,omitempty"`
}

func (r skillsSyncResult) Pretty(w io.Writer) {
	switch r.Action {
	case selfupdate.SkillsActionInstalled:
		fmt.Fprintf(w, "skill %s: installed successfully (%s)\n", r.Skill, r.Version)
	case selfupdate.SkillsActionUpdated:
		fmt.Fprintf(w, "skill %s: updated successfully (%s → %s)\n", r.Skill, r.PreviousVersion, r.Version)
	case selfupdate.SkillsActionRefreshed:
		fmt.Fprintf(w, "skill %s: refreshed successfully (%s)\n", r.Skill, r.Version)
	default:
		fmt.Fprintf(w, "skills: %s (%s)\n", r.Status, r.Version)
	}
}

func newSkillsSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Install or refresh all official Skills from this CLI release",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runSkillsSync(c.Context(), flagDryRun)
		},
	}
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Official Skills are missing, stale, or need repair without updating the CLI"},
		Prereq:   []string{"npx and the `skills` package are available"},
		Examples: []string{meta.BinName + " skills sync", meta.BinName + " skills sync --dry-run"},
	})
	return cmd
}

func runSkillsSync(parent context.Context, dryRun bool) error {
	result := skillsSyncResult{Skill: product.Current().OfficialSkillName, Version: meta.Version, Status: "would_sync"}
	if dryRun {
		out().Progress("dry-run: would synchronize official skills for %s", meta.Version)
		return out().Data(result)
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	out().Progress("synchronizing official skills for %s...", meta.Version)
	syncResult, err := selfupdate.SyncSkills(ctx, meta.Version)
	for _, warning := range syncResult.Warnings {
		out().Warn("%s", warning)
	}
	if err != nil {
		return errs.New("byteplus-rtc.skills.sync_failed", errs.TypeIO,
			"cannot synchronize official skills: %v", err).
			WithCause(err).
			WithHint("ensure `npx skills` is available, then retry `%s skills sync`", meta.BinName)
	}
	if syncResult.Skill != "" {
		result.Skill = syncResult.Skill
	}
	result.PreviousVersion = syncResult.PreviousVersion
	if syncResult.Version != "" {
		result.Version = syncResult.Version
	}
	result.Action = syncResult.Action
	result.Status = "synchronized"
	return out().Data(result)
}

func newSkillsListCmd() *cobra.Command {
	skillName := product.Current().OfficialSkillName
	cmd := &cobra.Command{
		Use:   "list [name[/path]]",
		Short: "List skills (name/description/version/references), or one layer under a skill",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 1 {
				return errs.New("byteplus-rtc.cli.invalid_flag", errs.TypeValidation,
					"list takes at most one argument: [name[/path]]").
					WithHint("run `%s skills list --help`", meta.BinName)
			}
			r, err := skillsReader()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				skills, err := r.List()
				if err != nil {
					return err
				}
				return out().Data(map[string]any{
					"skills": skills, "count": len(skills), "version": r.Version(),
				})
			}
			entries, listed, err := r.ListPath(args[0])
			if err != nil {
				return err
			}
			return out().Data(map[string]any{
				"path": listed, "entries": entries, "count": len(entries),
			})
		},
	}
	affordance.Attach(cmd, affordance.Affordance{
		When:     []string{"Discovering available skills and their reference files"},
		Examples: []string{meta.BinName + " skills list", meta.BinName + " skills list " + skillName},
	})
	return cmd
}

func newSkillsReadCmd() *cobra.Command {
	skillName := product.Current().OfficialSkillName
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "read <name>[/<path>] [path]",
		Short: "Print a skill's SKILL.md, or a reference under it (raw markdown by default)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			name, relpath, err := parseSkillReadTarget(args)
			if err != nil {
				return err
			}
			r, err := skillsReader()
			if err != nil {
				return err
			}
			var content []byte
			pathOut := "SKILL.md"
			if relpath == "" {
				content, err = r.ReadSkill(name)
			} else {
				content, pathOut, err = r.ReadReference(name, relpath)
			}
			if err != nil {
				return err
			}
			if asJSON || (c.Flags().Changed("format") && flagFormat == "json") {
				return out().Data(map[string]any{
					"skill": name, "path": pathOut,
					"version": r.Version(), "content": string(content),
				})
			}
			// Raw stdout stays byte-identical to the embedded source; the tip
			// goes to stderr so it never contaminates the markdown on stdout.
			if _, err := os.Stdout.Write(content); err != nil {
				return errs.Wrap(err, "byteplus-rtc.cli.internal", "write output: %s", err)
			}
			if pathOut == "SKILL.md" {
				fmt.Fprintf(os.Stderr,
					"# tip: browse this skill's files with `%s skills list %s` "+
						"(drill into subdirs via `%s/<dir>`), then read one with `%s skills read %s <path>`\n",
					meta.BinName, name, name, meta.BinName, name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit a JSON envelope instead of raw markdown")
	affordance.Attach(cmd, affordance.Affordance{
		When:   []string{"Loading a skill playbook or one of its references"},
		Prereq: []string{"Discover names/paths with `" + meta.BinName + " skills list`"},
		Examples: []string{
			meta.BinName + " skills read " + skillName,
			meta.BinName + " skills read " + skillName + " --json",
		},
	})
	return cmd
}

// parseSkillReadTarget maps 1-or-2 positional args to (name, relpath); a lone
// "<a>/<b>" splits on the first '/', and relpath "" reads the main SKILL.md.
func parseSkillReadTarget(args []string) (name, relpath string, err error) {
	switch len(args) {
	case 1:
		name, relpath = skillsfs.SplitArg(args[0])
		return name, relpath, nil
	case 2:
		if strings.ContainsAny(args[0], `/\`) {
			return "", "", errs.New("byteplus-rtc.cli.invalid_flag", errs.TypeValidation,
				"with two arguments the first must be a bare skill name, got %q", args[0]).
				WithHint("use `%s skills read <name> <path>` or `%s skills read <name>/<path>`", meta.BinName, meta.BinName)
		}
		return args[0], args[1], nil
	default:
		return "", "", errs.New("byteplus-rtc.cli.invalid_flag", errs.TypeValidation,
			"read requires 1 or 2 arguments: <name>[/<path>] [path]").
			WithHint("run `%s skills read --help`", meta.BinName)
	}
}
