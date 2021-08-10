package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/cli/safeexec"
)

func _main(args []string) error {
	if len(args) < 2 {
		return errors.New("Need an argument, either a username or an organization name")
	}

	// TODO determine if it's an org or a user

	// TODO get all repos for either a user or an org
	// TODO enumerate the repos
	// TODO see if there are workflows associated with each one
	// TODO
	// - fetch all workflows associated with an account (user or an org)
	// - report on pass/fail of last few run

}

func main() {
	err := _main(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, err)
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
