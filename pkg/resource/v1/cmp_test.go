package v1_test

import (
	"testing"

	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/google/go-cmp/cmp"
)

// TestVM ensures that v1.VM and its embedded structs can be compared
// using github.com/google/go-cmp/cmp without causing panics.
func TestVM(t *testing.T) {
	cmp.Equal(v1.VM{}, v1.VM{})
}
