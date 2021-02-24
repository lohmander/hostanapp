package plan

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type TestStateObject struct {
	name    string
	changed bool
}

func (o TestStateObject) ToString() string {
	return o.name
}

func (o TestStateObject) Changed() bool {
	return o.changed
}

func (o TestStateObject) Create() error { return nil }
func (o TestStateObject) Update() error { return nil }
func (o TestStateObject) Delete() error { return nil }

func makeTestObject(name string, changed bool) TestStateObject {
	return TestStateObject{
		name:    name,
		changed: changed,
	}
}

func TestPlan(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Plan Suite")
}

var _ = Describe("Make", func() {
	It("Should create a delete step", func() {
		x := makeTestObject("a", false)
		current := []TestStateObject{x}
		desired := []TestStateObject{}

		plan, err := Make(current, desired)

		Expect(err).Should(Succeed())
		Expect(plan.Steps[0].Action).Should(Equal(ActionDelete))
	})

	It("Should create a create step", func() {
		x := makeTestObject("a", false)
		current := []TestStateObject{}
		desired := []TestStateObject{x}

		plan, err := Make(current, desired)

		Expect(err).Should(Succeed())
		Expect(plan.Steps[0].Action).Should(Equal(ActionCreate))
	})

	It("Should create an update step", func() {
		x := makeTestObject("a", true)
		current := []TestStateObject{x}
		desired := []TestStateObject{x}

		plan, err := Make(current, desired)

		Expect(err).Should(Succeed())
		Expect(plan.Steps[0].Action).Should(Equal(ActionUpdate))
	})

	It("Should create an update and delete step", func() {
		x := makeTestObject("a", true)
		y := makeTestObject("b", false)
		z := makeTestObject("c", false)
		current := []TestStateObject{x, y, z}
		desired := []TestStateObject{x, z}

		plan, err := Make(current, desired)

		Expect(err).Should(Succeed())
		Expect(plan.Steps[0].Action).Should(Equal(ActionDelete))
		Expect(plan.Steps[1].Action).Should(Equal(ActionUpdate))
	})

	It("Should create a create step", func() {
		x := makeTestObject("a", true)
		current := []TestStateObject{}
		desired := []TestStateObject{x}

		plan, err := Make(current, desired)

		Expect(err).Should(Succeed())
		Expect(plan.Steps[0].Action).Should(Equal(ActionCreate))
	})
})
