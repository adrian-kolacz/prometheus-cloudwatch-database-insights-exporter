package pi

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	stypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/testutils"
)

type mockSTSClient struct {
	mu        sync.Mutex
	wasCalled bool
}

func (m *mockSTSClient) AssumeRole(_ context.Context, _ *sts.AssumeRoleInput, _ ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wasCalled = true
	return &sts.AssumeRoleOutput{
		Credentials: &stypes.Credentials{
			AccessKeyId:     aws.String("ASIAIOSFODNN7EXAMPLE"),
			SecretAccessKey: aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
			SessionToken:    aws.String("session-token"),
			Expiration:      aws.Time(time.Now().Add(time.Hour)),
		},
	}, nil
}

func (m *mockSTSClient) called() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.wasCalled
}

// verify mockSTSClient satisfies the interface at compile time
var _ stscreds.AssumeRoleAPIClient = (*mockSTSClient)(nil)

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

func TestNewPIClientWithSTSClient(t *testing.T) {
	t.Run("AssumeRole is called on first credential retrieval and returns expected credentials", func(t *testing.T) {
		mockSTS := &mockSTSClient{}
		cfg, err := config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(testutils.TestRegion),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "TOKEN")),
		)
		require.NoError(t, err)

		piClient, credCache := newPIClientWithSTSClient(cfg, "arn:aws:iam::123456789012:role/TestRole", mockSTS)
		require.NotNil(t, piClient)
		require.NotNil(t, piClient.client)

		assert.False(t, mockSTS.called(), "STS should not be called before credential retrieval")

		creds, err := credCache.Retrieve(context.TODO())
		require.NoError(t, err)

		assert.True(t, mockSTS.called(), "STS AssumeRole should have been called on credential retrieval")
		assert.Equal(t, "ASIAIOSFODNN7EXAMPLE", creds.AccessKeyID)
		assert.Equal(t, "session-token", creds.SessionToken)
	})
}
