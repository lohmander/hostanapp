package sliceutils

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type StringEntity string

func (s StringEntity) ToString() string {
	return fmt.Sprintf("%s", s)
}

type StructEntity struct {
	name string
}

func (s StructEntity) ToString() string {
	return s.name
}

func TestSliceUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Slice utils Suite")
}

var _ = Describe("Includes", func() {
	It("Should find element in slice", func() {
		x := []StringEntity{"a", "b", "c"}
		var y StringEntity = "b"

		Expect(Includes(x, y)).Should(BeTrue())
	})

	It("Should not find element in slice", func() {
		x := []StringEntity{"a", "b", "c"}
		var y StringEntity = "d"

		Expect(Includes(x, y)).Should(BeFalse())
	})
})

var _ = Describe("Subtract", func() {
	It("Should remove elements of y from x", func() {
		x := []StringEntity{"a", "b", "c"}
		y := []StringEntity{"a", "c"}

		Expect(Subtract(x, y)).Should(BeEquivalentTo([]StringEntity{"b"}))
	})

	It("Should ignore elements of y not in x", func() {
		x := []StringEntity{"a", "b", "c"}
		y := []StringEntity{"x", "y", "z"}

		Expect(Subtract(x, y)).Should(BeEquivalentTo([]StringEntity{"a", "b", "c"}))
	})

	It("Should work with empty y", func() {
		x := []StringEntity{"a"}
		y := []StringEntity{}

		Expect(Subtract(x, y)).Should(BeEquivalentTo([]StringEntity{"a"}))
	})

	It("Should work with struct entities", func() {
		x := []StructEntity{{"a"}, {"b"}, {"c"}}
		y := []StructEntity{{"b"}}

		Expect(Subtract(x, y)).Should(BeEquivalentTo([]StructEntity{{"a"}, {"c"}}))
	})

	It("Should work with struct entities and empty y", func() {
		x := []StructEntity{{"a"}}
		y := []StructEntity{}

		Expect(Subtract(x, y)).Should(BeEquivalentTo([]StructEntity{{"a"}}))
	})
})

var _ = Describe("Intersection", func() {
	It("Should find the intersection betweeen x and y", func() {
		x := []StringEntity{"a", "b", "c"}
		y := []StringEntity{"a", "c"}

		Expect(Intersection(x, y)).Should(BeEquivalentTo([]StringEntity{"a", "c"}))
	})

	It("Should produce an empty slice if no elements intersect", func() {
		x := []StringEntity{"a", "b", "c"}
		y := []StringEntity{"x", "y", "z"}

		Expect(Intersection(x, y)).Should(BeEquivalentTo([]StringEntity{}))
	})
})
