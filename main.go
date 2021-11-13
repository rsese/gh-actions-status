package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"golang.org/x/term"

	"github.com/charmbracelet/lipgloss"
	"github.com/cli/safeexec"
)

const defaultMaxRuns = 5
const defaultWorkflowNameLength = 17
const defaultApiCacheTime = "60m"

/*
TODO accept month as flag argument
TODO bring in lipgloss to render stuff; for now, render a big list
*/

type run struct {
	Finished   time.Time
	Elapsed    time.Duration
	Status     string
	Conclusion string
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

func _main(args []string) error {
	if len(args) < 2 {
		return errors.New("Need an argument, either a username or an organization name")
	}

	selector := args[1]

	var repos []*repositoryData
	var orgErr error
	var userErr error

	repos, orgErr = getReposForOrg(selector)
	if orgErr != nil {
		repos, userErr = getReposForUser(selector)
		if userErr != nil {
			// TODO nicer error handling
			return errors.New("oh no")
		}
	}

	columnWidth := defaultWorkflowNameLength + 3 // +3 for "..."
	cardsPerRow := getTerminalWidth() / columnWidth

	// TODO card style
	cardStyle := lipgloss.NewStyle().
		Align(lipgloss.Left).
		Width(columnWidth)

	fmt.Printf("GitHub Actions dashboard for %s for the month of %s\n", selector, "TODO")

	totalBillableMs := 0

	for _, r := range repos {
		workflows, err := getWorkflows(*r)
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
		// TODO compute and print repo stats
		renderedCards := []string{}
		for _, w := range r.Workflows {
			renderedCards = append(renderedCards, cardStyle.Render(w.RenderCard()))
		}
		fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top, renderedCards...))
	}

	// TODO lipgloss tasks:
	// - border around cards
	// - default to max four columns
	// - snug up any margin/padding
	// - add embellishments like color/bolding

	// TODO recognize if we're looking for the authenticated user, uses a different endpoint

	// gh api "/orgs/cli/repos" --jq ".[]|.full_name"

	return nil
}

func getReposForOrg(selector string) ([]*repositoryData, error) {
	s := fmt.Sprintf("orgs/%s/repos", selector)

	return getRepos(s)
}

func getReposForUser(selector string) ([]*repositoryData, error) {
	s := fmt.Sprintf("users/%s/repos", selector)

	return getRepos(s)
}

func getRepos(path string) ([]*repositoryData, error) {
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

func getWorkflows(repoData repositoryData) ([]*workflow, error) {
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
		CreatedAt  time.Time `json:"created_at"`
		UpdatedAt  time.Time `json:"updated_at"`
		Status     string
		Conclusion string
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
			rr := run{Status: r.Status, Conclusion: r.Conclusion}

			if r.Status == "completed" {
				rr.Finished = r.UpdatedAt
				rr.Elapsed = r.UpdatedAt.Sub(r.CreatedAt)
			}

			runs = append(runs, rr)
		}

		billablePath := fmt.Sprintf("%s/timing", w.URL)
		stdout, _, err = gh("api", "--cache", defaultApiCacheTime, billablePath, "--jq", ".billable")

		if repoData.Private {
			bp := billablePayload{}
			err = json.Unmarshal(stdout.Bytes(), &bp)
			if err != nil {
				return nil, err
			}

			totalMs += bp.MacOs.TotalMs + bp.Windows.TotalMs + bp.Ubuntu.TotalMs
		}

		out = append(out, &workflow{
			Name:       w.Name,
			Runs:       runs,
			BillableMs: totalMs,
		})
	}

	return out, nil
}

func main() {
	err := _main(os.Args)
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
