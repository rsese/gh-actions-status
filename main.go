package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"

	"golang.org/x/term"

	"github.com/charmbracelet/lipgloss"
	"github.com/cli/safeexec"
	flag "github.com/spf13/pflag"
)

const defaultMaxRuns = 5
const defaultWorkflowNameLength = 17
const defaultApiCacheTime = "60m"

/*
	// TODO
	// * add link to repo Actions tab
	// * add flag for time period (e.g pick a month or start/end date)
	// * UI updates (icon colors, bold repo name, etc.)

	// TODO recognize if we're looking for the authenticated user, uses a different endpoint
	// - is this actually important?
	// gh api "/orgs/cli/repos" --jq ".[]|.full_name"


	// --last 90d

*/

type run struct {
	Finished   time.Time
	Elapsed    time.Duration
	Status     string
	Conclusion string
	URL        string
}

type workflow struct {
	Name       string
	Runs       []run
	BillableMs int
}

func (w *workflow) RenderHealth() string {
	var results string

	for i, r := range w.Runs {
		if i > defaultMaxRuns {
			break
		}

		if r.Status != "completed" {
			results += "-"
			continue
		}

		switch r.Conclusion {
		case "success":
			results += "✓"
		case "skipped", "cancelled", "neutral":
			results += "-"
		default:
			results += "x"
		}
	}

	return results
}

func (w *workflow) AverageElapsed() time.Duration {
	var totalTime int
	var averageTime int

	for i, r := range w.Runs {
		if i > defaultMaxRuns {
			break
		}

		totalTime += int(r.Elapsed.Seconds())
	}

	averageTime = totalTime / defaultMaxRuns

	s := fmt.Sprintf("%ds", averageTime)
	d, _ := time.ParseDuration(s)

	return d
}

func truncateWorkflowName(name string, length int) string {
	if len(name) > length {
		return name[:length] + "..."
	}

	return name
}

func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))

	if err != nil {
		panic(err.Error())
	}

	return width
}

func (w *workflow) RenderCard() string {
	// Assumes that run data is time filtered already
	tmpl, _ := template.New("workflowCard").Parse(
		`{{ .Name }}
Health: {{ .Health }}
Avg elapsed: {{ .AvgElapsed }}
{{- if .BillableMs }}
Billable time: {{call .PrettyMS .BillableMs }}{{end}}`)

	tmplData := struct {
		Name       string
		AvgElapsed time.Duration
		Health     string
		BillableMs int
		PrettyMS   func(int) string
	}{
		Name:       truncateWorkflowName(w.Name, defaultWorkflowNameLength),
		AvgElapsed: w.AverageElapsed(),
		Health:     w.RenderHealth(),
		BillableMs: w.BillableMs,
		PrettyMS:   prettyMS,
	}

	buf := bytes.Buffer{}
	_ = tmpl.Execute(&buf, tmplData)
	return buf.String()
}

type repositoryData struct {
	Name      string `json:"full_name"`
	Private   bool
	Workflows []*workflow
}

func prettyMS(ms int) string {
	if ms == 60000 {
		return fmt.Sprintf("1m")
	}
	if ms < 60000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.2fm", float32(ms)/60000)
}

func daysToHours(days string) string {
	timeUnit := string(days[len(days)-1])

	if timeUnit == "h" {
		return days
	}

	if timeUnit != "d" {
		fmt.Fprintf(os.Stderr, "Period of time should be in days, e.g. '7d' for 7 days\n")
		os.Exit(1)
	}

	daysAsString := days[:len(days)-1]
	daysNum, err := strconv.ParseInt(daysAsString, 0, 32)

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	return fmt.Sprintf("%dh", daysNum*24)
}

type options struct {
	Repositories []string
	Last         time.Duration
	Selector     string
}

func _main(opts *options) error {
	selector := opts.Selector
	repositories := opts.Repositories
	last := opts.Last

	var repos []*repositoryData

	if len(repositories) > 0 {
		var path string
		var stdout bytes.Buffer
		var err error
		var data repositoryData
		for _, repoName := range repositories {
			path = fmt.Sprintf("repos/%s/%s", selector, repoName)
			if stdout, _, err = gh("api", "--cache", defaultApiCacheTime, path); err != nil {
				return err
			}
			if err = json.Unmarshal(stdout.Bytes(), &data); err != nil {
				return err
			}
			repos = append(repos, &data)
		}
	} else {
		var orgErr error
		var userErr error
		repos, orgErr = getAllReposForOrg(selector)
		if orgErr != nil {
			repos, userErr = getAllReposForUser(selector)
			if userErr != nil {
				return fmt.Errorf("could not find '%s': %w; %w", selector, orgErr, userErr)
			}
		}
	}

	columnWidth := defaultWorkflowNameLength + 3 // +3 for "..."
	cardsPerRow := getTerminalWidth() / columnWidth

	cardStyle := lipgloss.NewStyle().
		Align(lipgloss.Left).
		Padding(1).
		Width(columnWidth).
		BorderStyle(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("63"))

	fmt.Printf("GitHub Actions dashboard for %s for the month of %s\n", selector, "TODO")

	totalBillableMs := 0

	for _, r := range repos {
		workflows, err := getWorkflows(*r, last)
		if err != nil {
			return err
		}

		r.Workflows = workflows

		for _, w := range workflows {
			totalBillableMs += w.BillableMs
		}
	}

	fmt.Printf("Total billable time: %s\n", prettyMS(totalBillableMs))

	for _, r := range repos {
		if len(r.Workflows) == 0 {
			continue
		}
		fmt.Println()
		fmt.Println(r.Name)

		totalRows := int(math.Ceil(float64(len(r.Workflows)) / float64(cardsPerRow)))
		cardRows := make([][]string, totalRows)
		rowIndex := 0

		for _, w := range r.Workflows {
			if len(cardRows[rowIndex]) == cardsPerRow {
				rowIndex++
			}

			cardRows[rowIndex] = append(cardRows[rowIndex], cardStyle.Render(w.RenderCard()))
		}

		for _, row := range cardRows {
			fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top, row...))
		}
	}

	return nil
}

func getAllReposForOrg(selector string) ([]*repositoryData, error) {
	s := fmt.Sprintf("orgs/%s/repos", selector)

	return getAllRepos(s)
}

func getAllReposForUser(selector string) ([]*repositoryData, error) {
	s := fmt.Sprintf("users/%s/repos", selector)

	return getAllRepos(s)
}

func getAllRepos(path string) ([]*repositoryData, error) {
	stdout, _, err := gh("api", "--cache", defaultApiCacheTime, path)
	if err != nil {
		return nil, err
	}

	repoData := []*repositoryData{}
	err = json.Unmarshal(stdout.Bytes(), &repoData)
	if err != nil {
		return nil, err
	}

	return repoData, nil
}

func getWorkflows(repoData repositoryData, last time.Duration) ([]*workflow, error) {
	workflowsPath := fmt.Sprintf("repos/%s/actions/workflows", repoData.Name)

	stdout, _, err := gh("api", "--cache", defaultApiCacheTime, workflowsPath, "--jq", ".workflows")
	if err != nil {
		return nil, err
	}

	type workflowsPayload struct {
		Id    int `json:"id"`
		State string
		Name  string
		URL   string `json:"url"`
	}

	p := []workflowsPayload{}
	err = json.Unmarshal(stdout.Bytes(), &p)
	if err != nil {
		return nil, err
	}

	out := []*workflow{}

	type runPayload struct {
		Id         int       `json:"id"`
		CreatedAt  time.Time `json:"created_at"`
		UpdatedAt  time.Time `json:"updated_at"`
		Status     string
		Conclusion string
		URL        string
	}

	type billablePayload struct {
		MacOs struct {
			TotalMs int `json:"total_ms"`
		} `json:"MACOS"`
		Windows struct {
			TotalMs int `json:"total_ms"`
		} `json:"WINDOWS"`
		Ubuntu struct {
			TotalMs int `json:"total_ms"`
		} `json:"UBUNTU"`
	}

	var totalMs int

	for _, w := range p {
		if strings.HasPrefix(w.State, "disabled") {
			continue
		}

		runsPath := fmt.Sprintf("%s/runs", w.URL)
		stdout, _, err = gh("api", "--cache", defaultApiCacheTime, runsPath, "--jq", ".workflow_runs")
		rs := []runPayload{}
		err = json.Unmarshal(stdout.Bytes(), &rs)
		if err != nil {
			return nil, err
		}

		runs := []run{}

		for _, r := range rs {
			rr := run{Status: r.Status, Conclusion: r.Conclusion, URL: r.URL}

			if r.Status == "completed" {
				rr.Finished = r.UpdatedAt
				rr.Elapsed = r.UpdatedAt.Sub(r.CreatedAt)
				finishedAgo := time.Now().Sub(rr.Finished)

				if last-finishedAgo > 0 {
					runs = append(runs, rr)
				}
			}
		}

		if repoData.Private {
			for _, r := range runs {
				runTimingPath := fmt.Sprintf("%s/timing", r.URL)
				stdout, _, err = gh("api", "--cache", defaultApiCacheTime, runTimingPath, "--jq", ".billable")
				bp := billablePayload{}
				err = json.Unmarshal(stdout.Bytes(), &bp)
				if err != nil {
					return nil, err
				}

				totalMs += bp.MacOs.TotalMs + bp.Windows.TotalMs + bp.Ubuntu.TotalMs
			}
		}

		out = append(out, &workflow{
			Name:       w.Name,
			Runs:       runs,
			BillableMs: totalMs,
		})
	}

	return out, nil
}

func parseArgs() (*options, error) {
	repositories := flag.StringSliceP("repos", "r", []string{}, "One or more repository names from the provided org or user")
	last := flag.StringP("last", "l", "30d", "What period of time to cover. Default: 30d")

	flag.Parse()

	if len(flag.Args()) != 1 {
		return nil, errors.New("need exactly one argument, either an organization or user name")
	}

	hoursLast, err := time.ParseDuration(daysToHours(*last))

	if err != nil {
		return nil, fmt.Errorf("failed to parse duration: %w", err)
	}

	return &options{
		Repositories: *repositories,
		Last:         hoursLast,
		Selector:     flag.Arg(0),
	}, nil
}

func main() {
	opts, err := parseArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse arguments: %s\n", err)
		os.Exit(1)
	}

	// TODO testing is annoying bc of flag.Parse() in _main
	err = _main(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

// gh shells out to gh, returning STDOUT/STDERR and any error
func gh(args ...string) (sout, eout bytes.Buffer, err error) {
	ghBin, err := safeexec.LookPath("gh")
	if err != nil {
		err = fmt.Errorf("could not find gh. Is it installed? error: %w", err)
		return
	}

	cmd := exec.Command(ghBin, args...)
	cmd.Stderr = &eout
	cmd.Stdout = &sout

	err = cmd.Run()
	if err != nil {
		err = fmt.Errorf("failed to run gh. error: %w, stderr: %s", err, eout.String())
		return
	}

	return
}
