package v1_test

import (
	v1 "github.com/cirruslabs/orchard/pkg/resource/v1"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewRestartPolicyFromString(t *testing.T) {
	_, err := v1.NewRestartPolicyFromString("")
	assert.Error(t, err, "empty restart policy should yield an error")

	_, err = v1.NewRestartPolicyFromString("non-existent")
	assert.Error(t, err, "non-existent restart policy should yield an error")

	_, err = v1.NewRestartPolicyFromString("never")
	assert.Error(t, err, "improperly capitalized but existent policy should yield an error")

	restartPolicy, err := v1.NewRestartPolicyFromString("Never")
	assert.NoError(t, err, "Never policy should be parsed correctly")
	assert.Equal(t, v1.RestartPolicyNever, restartPolicy)

	restartPolicy, err = v1.NewRestartPolicyFromString("OnFailure")
	assert.NoError(t, err, "OnFailure policy should be parsed correctly")
	assert.Equal(t, v1.RestartPolicyOnFailure, restartPolicy)
}
