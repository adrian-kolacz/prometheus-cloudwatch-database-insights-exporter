package pi

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/testutils"
)

func TestNewPIClient(t *testing.T) {
	t.Run("creates new PI client successfully", func(t *testing.T) {
		piClient, err := NewPIClient(testutils.TestRegion)
		assert.NoError(t, err)
		assert.NotNil(t, piClient)
		assert.NotNil(t, piClient.client)
	})
}

func TestNewPIClientWithRole(t *testing.T) {
	t.Run("empty roleARN behaves like NewPIClient", func(t *testing.T) {
		piClient, err := NewPIClientWithRole(testutils.TestRegion, "")
		assert.NoError(t, err)
		assert.NotNil(t, piClient)
		assert.NotNil(t, piClient.client)
	})

	t.Run("non-empty roleARN constructs client without network calls", func(t *testing.T) {
		// AssumeRole is lazy — STS is not called until the first API request,
		// so construction succeeds even without reachable AWS credentials.
		piClient, err := NewPIClientWithRole(testutils.TestRegion, "arn:aws:iam::123456789012:role/TestRole")
		assert.NoError(t, err)
		assert.NotNil(t, piClient)
		assert.NotNil(t, piClient.client)
	})
}
