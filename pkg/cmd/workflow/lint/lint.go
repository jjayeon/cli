package lint

import (
	"fmt"
	"net/http"
	// "strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	// runShared "github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	// "github.com/cli/cli/v2/pkg/markdown"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"

	"github.com/actions-mlh/go-workflows/lint"
)

type LintOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    cmdutil.Browser

	Selector string
	Ref      string
	Web      bool
	Prompt   bool
	Raw      bool
	YAML     bool
}

func NewCmdLint(f *cmdutil.Factory, runF func(*LintOptions) error) *cobra.Command {
	opts := &LintOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "lint [<workflow-id> | <workflow-name> | <filename>]",
		Short: "Lint a workflow",
		Args:  cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
		  # BAD INFO rewrite later
		  # Interactively select a workflow to view
		  $ gh workflow view

		  # View a specific workflow
		  $ gh workflow view 0451
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.Raw = !opts.IO.IsStdoutTTY()

			if len(args) > 0 {
				opts.Selector = args[0]
			} else if !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("workflow argument required when not running interactively")
			} else {
				opts.Prompt = true
			}

			if !opts.YAML && opts.Ref != "" {
				return cmdutil.FlagErrorf("`--yaml` required when specifying `--ref`")
			}

			if runF != nil {
				return runF(opts)
			}
			return runView(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open workflow in the browser")
	cmd.Flags().BoolVarP(&opts.YAML, "yaml", "y", false, "View the workflow yaml file")
	cmd.Flags().StringVarP(&opts.Ref, "ref", "r", "", "The branch or tag name which contains the version of the workflow file you'd like to view")

	return cmd
}

func runView(opts *LintOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	var workflow *shared.Workflow
	states := []shared.WorkflowState{shared.Active}
	workflow, err = shared.ResolveWorkflow(opts.IO, client, repo, opts.Prompt, opts.Selector, states)
	if err != nil {
		return err
	}

	if opts.Web {
		var url string
		if opts.YAML {
			ref := opts.Ref
			if ref == "" {
				opts.IO.StartProgressIndicator()
				ref, err = api.RepoDefaultBranch(client, repo)
				opts.IO.StopProgressIndicator()
				if err != nil {
					return err
				}
			}
			url = ghrepo.GenerateRepoURL(repo, "blob/%s/%s", ref, workflow.Path)
		} else {
			url = ghrepo.GenerateRepoURL(repo, "actions/workflows/%s", workflow.Base())
		}
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", utils.DisplayURL(url))
		}
		return opts.Browser.Browse(url)
	}

	err = viewWorkflowContent(opts, client, workflow)
	if err != nil {
		return err
	}

	return nil
}

func viewWorkflowContent(opts *LintOptions, client *api.Client, workflow *shared.Workflow) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	opts.IO.StartProgressIndicator()
	yamlBytes, err := shared.GetWorkflowContent(client, repo, *workflow, opts.Ref)
	opts.IO.StopProgressIndicator()
	if err != nil {
		if s, ok := err.(api.HTTPError); ok && s.StatusCode == 404 {
			if opts.Ref != "" {
				return fmt.Errorf("could not find workflow file %s on %s, try specifying a different ref", workflow.Base(), opts.Ref)
			}
			return fmt.Errorf("could not find workflow file %s, try specifying a branch or tag using `--ref`", workflow.Base())
		}
		return fmt.Errorf("could not get workflow file content: %w", err)
	}
	
	problems, err := lint.Lint(workflow.Name, yamlBytes)
	if err != nil {
		return fmt.Errorf("error linting file %s: %s", workflow.Name, err)
	}

	for problem := range problems {
		fmt.Fprintln(opts.IO.Out, problem)
	}
	// -
	
	// yaml := string(yamlBytes)

	// theme := opts.IO.DetectTerminalTheme()
	// markdownStyle := markdown.GetStyle(theme)
	// if err := opts.IO.StartPager(); err != nil {
	// 	fmt.Fprintf(opts.IO.ErrOut, "starting pager failed: %v\n", err)
	// }
	// defer opts.IO.StopPager()

	// if !opts.Raw {
	// 	cs := opts.IO.ColorScheme()
	// 	out := opts.IO.Out

	// 	fileName := workflow.Base()
	// 	fmt.Fprintf(out, "%s - %s\n", cs.Bold(workflow.Name), cs.Gray(fileName))
	// 	fmt.Fprintf(out, "ID: %s", cs.Cyanf("%d", workflow.ID))

	// 	codeBlock := fmt.Sprintf("```yaml\n%s\n```", yaml)
	// 	rendered, err := markdown.RenderWithOpts(codeBlock, markdownStyle,
	// 		markdown.RenderOpts{
	// 			markdown.WithoutIndentation(),
	// 			markdown.WithoutWrap(),
	// 		})
	// 	if err != nil {
	// 		return err
	// 	}
	// 	_, err = fmt.Fprint(opts.IO.Out, rendered)
	// 	return err
	// }

	// if _, err := fmt.Fprint(opts.IO.Out, yaml); err != nil {
	// 	return err
	// }

	// if !strings.HasSuffix(yaml, "\n") {
	// 	_, err := fmt.Fprint(opts.IO.Out, "\n")
	// 	return err
	// }

	return nil
}
