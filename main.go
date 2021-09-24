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

	"github.com/cli/safeexec"
)

const defaultMaxRuns = 5

/*
TODO get billable minutes
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
	Name string
	Runs []run
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

func (w *workflow) RenderCard() string {
	// Assumes that run data is time filtered already
	tmpl, _ := template.New("workflowCard").Parse(
		`{{ .Name }}
Avg elapsed:   {{ .AvgElapsed }}
Health: {{ .Health }}`)

	tmplData := struct {
		Name       string
		AvgElapsed time.Duration
		Health     string
	}{
		Name:       w.Name,
		AvgElapsed: w.AverageElapsed(),
		Health:     w.RenderHealth(),
	}

	buf := bytes.Buffer{}
	_ = tmpl.Execute(&buf, tmplData)
	return buf.String()
}

type repositoryData struct {
	Name      string
	Workflows []*workflow
}

func _main(args []string) error {
	if len(args) < 2 {
		return errors.New("Need an argument, either a username or an organization name")
	}

	selector := args[1]

	var repos []string
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

	data := []repositoryData{}

	for _, r := range repos {
		workflows, err := getWorkflows(r)
		if err != nil {
			return err
		}

		repoData := repositoryData{
			Name:      r,
			Workflows: workflows,
		}

		data = append(data, repoData)
	}

	for _, r := range data {
		if len(r.Workflows) == 0 {
			continue
		}
		// TODO print repo header
		// TODO compute repo stats
		fmt.Println()
		fmt.Println(r.Name)
		for _, w := range r.Workflows {
			fmt.Println(w.RenderCard())
		}
	}

	// TODO report on pass/fail of last few run
	// TODO recognize if we're looking for the authenticated user, uses a different endpoint

	// gh api "/orgs/cli/repos" --jq ".[]|.full_name"

	return nil
}

func getReposForOrg(selector string) ([]string, error) {
	s := fmt.Sprintf("orgs/%s/repos", selector)

	return getRepos(s)
}

func getReposForUser(selector string) ([]string, error) {
	s := fmt.Sprintf("users/%s/repos", selector)

	return getRepos(s)
}

func getRepos(path string) ([]string, error) {
	stdout, _, err := gh("api", "--cache", "5m", path, "--jq", ".[] | .full_name")

	if err != nil {
		return nil, err
	}

	repos := strings.Split(stdout.String(), "\n")

	// TODO filter list and remove ""
	return repos[0 : len(repos)-1], nil
}

func getWorkflows(repo string) ([]*workflow, error) {
	workflowsPath := fmt.Sprintf("repos/%s/actions/workflows", repo)

	stdout, _, err := gh("api", "--cache", "5m", workflowsPath, "--jq", ".workflows")
	if err != nil {
		return nil, err
	}

	type workflowsPayload struct {
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

	for _, w := range p {
		if strings.HasPrefix(w.State, "disabled") {
			continue
		}

		runsPath := fmt.Sprintf("%s/runs", w.URL)
		stdout, _, err = gh("api", "--cache", "5m", runsPath, "--jq", ".workflow_runs")
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

		out = append(out, &workflow{
			Name: w.Name,
			Runs: runs,
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
