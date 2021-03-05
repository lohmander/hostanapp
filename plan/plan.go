package plan

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/lohmander/hostanapp/sliceutils"
	"github.com/thoas/go-funk"
)

// Resource interface
type Resource interface {
	sliceutils.ToString

	Changed() bool
	Create() error
	Update() error
	Delete() error
}

// Action type
type Action int

const (
	// ActionCreate is a plan step to create a new object
	ActionCreate Action = iota

	// ActionUpdate is a plan step to update an existing object
	ActionUpdate

	// ActionDelete is a plan step to delete an existing object
	ActionDelete
)

// Step is one step of a bigger plan
type Step struct {
	Action Action
	Object Resource
	Reason string
}

// Plan is a sequence of steps to reconcile two slices of states
type Plan struct {
	Steps []Step
}

// Make makes a new plan based on two state slices
func Make(current, desired interface{}) (*Plan, error) {
	steps := []Step{}
	removed := sliceutils.Subtract(current, desired)
	present := sliceutils.Intersection(current, desired)
	added := sliceutils.Subtract(desired, current)

	funk.ForEach(removed, func(x Resource) {
		steps = append(steps, Step{ActionDelete, x, "Not present in the desired states"})
	})

	funk.ForEach(present, func(x Resource) {
		if x.Changed() {
			steps = append(steps, Step{ActionUpdate, x, "Configuration changed"})
		}
	})

	funk.ForEach(added, func(x Resource) {
		steps = append(steps, Step{ActionCreate, x, "Not present in current states"})
	})

	return &Plan{steps}, nil
}

// Describe describes the plan
func (p *Plan) Describe() string {
	steps := []string{}
	actions := map[Action]string{
		ActionDelete: color.New(color.BgRed, color.FgBlack).SprintFunc()("DELETE"),
		ActionUpdate: color.New(color.BgCyan, color.FgBlack).SprintFunc()("UPDATE"),
		ActionCreate: color.New(color.BgGreen, color.FgBlack).SprintFunc()("CREATE"),
	}

	for _, step := range p.Steps {
		steps = append(steps, fmt.Sprintf("%s: %s\t\t– %s", actions[step.Action], step.Object.ToString(), step.Reason))
	}

	return fmt.Sprintf("\n%s\n", strings.Join(steps, "\n"))
}

// Execute executes a plan
func (p *Plan) Execute() error {
	for _, step := range p.Steps {
		var exec func() error

		if step.Action == ActionCreate {
			exec = step.Object.Create
		}

		if step.Action == ActionUpdate {
			exec = step.Object.Update
		}

		if step.Action == ActionDelete {
			exec = step.Object.Delete
		}

		if err := exec(); err != nil {
			return err
		}
	}

	return nil
}
