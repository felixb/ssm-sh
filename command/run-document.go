package command

import (
	"context"
	"fmt"
	"github.com/itsdalmo/ssm-sh/manager"
	"github.com/pkg/errors"
	"os"
	"time"
)

// RunDocumentCommand contains all arguments for run-document command
type RunDocumentCommand struct {
	Name       string            `short:"n" long:"name" description:"Name of document in ssm."`
	Timeout    int               `short:"i" long:"timeout" description:"Seconds to wait for command result before timing out." default:"30"`
	Parameters map[string]string `short:"p" long:"parameter" description:"Zero or more parameters for the document (name:value)"`
	SSMOpts    SSMOptions        `group:"SSM options"`
	TargetOpts TargetOptions
}

// Execute run-document command
func (command *RunDocumentCommand) Execute(args []string) error {
	if command.Name == "" {
		return errors.New("No document name set to trigger")
	}

	sess, err := newSession()
	if err != nil {
		return errors.Wrap(err, "failed to create new aws session")
	}

	opts, err := command.SSMOpts.Parse()
	if err != nil {
		return err
	}
	m := manager.NewManager(sess, Command.AwsOpts.Region, *opts)
	targets, err := setTargets(command.TargetOpts)
	if err != nil {
		return errors.Wrap(err, "failed to set targets")
	}
	fmt.Printf("Use ctrl-c to abort the command early.\n\n")

	// Start the command
	commandID, err := m.RunCommand(targets, command.Name, command.Parameters)
	if err != nil {
		return errors.Wrap(err, "failed to run command")
	}

	// Catch sigterms to gracefully shut down
	var interrupts int
	abort := interruptHandler()

	// Get output
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(command.Timeout)*time.Second)
	defer cancel()

	out := make(chan *manager.CommandOutput)
	go m.GetCommandOutput(ctx, targets, commandID, out)

	for {
		select {
		case <-ctx.Done():
			return errors.New("timeout reached")
		case <-abort:
			interrupts++
			err := m.AbortCommand(targets, commandID)
			if err != nil {
				return errors.Wrap(err, "failed to abort command on sigterm")
			}
			if interrupts > 1 {
				return errors.New("interrupted by user")
			}
		case output, open := <-out:
			if !open {
				return nil
			}
			err := PrintCommandOutput(os.Stdout, output)
			if err != nil {
				return errors.Wrap(err, "failed to print output")
			}
		}
	}
}
