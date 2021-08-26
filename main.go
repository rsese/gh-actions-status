package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cli/safeexec"
)

type run struct {
	Finished   time.Time
	Elapsed    time.Duration
	Status     string
	Conclusion string
}

type workflow struct {
	Name string
	Run  []run
}

func (w *workflow) OverallHealth() string {
	// TODO enum for green yellow red
	return "passing"
}

func (w *workflow) AverageElapsed() time.Duration {
	// TODO
	d, _ := time.ParseDuration("1s")
	return d
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

	fmt.Printf("DBG %#v\n", repos)

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

	fmt.Printf("DBG %#v\n", data)

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
	stdout, _, err := gh("api", path, "--jq", ".[] | .full_name")

	if err != nil {
		return nil, err
	}

	repos := strings.Split(stdout.String(), "\n")

	// TODO filter list and remove ""
	return repos[0 : len(repos)-1], nil
}

func getWorkflows(repo string) ([]*workflow, error) {
	s := fmt.Sprintf("repos/%s/actions/workflows", repo)

	stdout, _, err := gh("api", s, "--jq", ".workflows")
	if err != nil {
		return nil, err
	}

	type payload struct {
		State string
		Name  string
	}

	p := []payload{}
	err = json.Unmarshal(stdout.Bytes(), &p)
	if err != nil {
		return nil, err
	}

	out := []*workflow{}
	for _, w := range p {
		if strings.HasPrefix(w.State, "disabled") {
			continue
		}
		// TODO actually get run info
		out = append(out, &workflow{
			Name: w.Name,
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
