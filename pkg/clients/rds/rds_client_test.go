package rds

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

func TestNewRDSClient(t *testing.T) {
	t.Run("creates new RDS client successfully", func(t *testing.T) {
		rdsClient, err := NewRDSClient(testutils.TestRegion)
		assert.NoError(t, err)
		assert.NotNil(t, rdsClient)
		assert.NotNil(t, rdsClient.client)
	})

	t.Run("creates new RDS client with valid region", func(t *testing.T) {
		regions := []string{"us-west-2", "us-east-1", "eu-west-1"}
		for _, region := range regions {
			rdsClient, err := NewRDSClient(region)
			assert.NoError(t, err)
			assert.NotNil(t, rdsClient)
			assert.NotNil(t, rdsClient.client)
		}
	})
}

func TestNewRDSClientWithRole(t *testing.T) {
	t.Run("empty roleARN behaves like NewRDSClient", func(t *testing.T) {
		rdsClient, err := NewRDSClientWithRole(testutils.TestRegion, "")
		assert.NoError(t, err)
		assert.NotNil(t, rdsClient)
		assert.NotNil(t, rdsClient.client)
	})

	t.Run("non-empty roleARN constructs client without network calls", func(t *testing.T) {
		// AssumeRole is lazy — STS is not called until the first API request,
		// so construction succeeds even without reachable AWS credentials.
		rdsClient, err := NewRDSClientWithRole(testutils.TestRegion, "arn:aws:iam::123456789012:role/TestRole")
		assert.NoError(t, err)
		assert.NotNil(t, rdsClient)
		assert.NotNil(t, rdsClient.client)
	})
}

func TestNewRDSClientWithSTSClient(t *testing.T) {
	t.Run("AssumeRole is called on first credential retrieval and returns expected credentials", func(t *testing.T) {
		mockSTS := &mockSTSClient{}
		cfg, err := config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(testutils.TestRegion),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "TOKEN")),
		)
		require.NoError(t, err)

		rdsClient, credCache := newRDSClientWithSTSClient(cfg, "arn:aws:iam::123456789012:role/TestRole", mockSTS)
		require.NotNil(t, rdsClient)
		require.NotNil(t, rdsClient.client)

		assert.False(t, mockSTS.called(), "STS should not be called before credential retrieval")

		creds, err := credCache.Retrieve(context.TODO())
		require.NoError(t, err)

		assert.True(t, mockSTS.called(), "STS AssumeRole should have been called on credential retrieval")
		assert.Equal(t, "ASIAIOSFODNN7EXAMPLE", creds.AccessKeyID)
		assert.Equal(t, "session-token", creds.SessionToken)
	})
}

func TestDescribeDBInstancesPaginatorIntegration(t *testing.T) {
	testCases := []struct {
		name            string
		region          string
		expectError     bool
		skipIntegration bool
	}{
		{
			name:            "integration test - describe instances with pagination in us-west-2",
			region:          "us-west-2",
			expectError:     false,
			skipIntegration: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipIntegration {
				t.Skip("Skipping integration test - requires AWS credentials and actual RDS instances")
			}

			rdsClient, err := NewRDSClient(tc.region)
			assert.NoError(t, err)

			instances, err := rdsClient.DescribeDBInstancesPaginator(context.Background())
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, instances)
				t.Logf("Retrieved %d DB instances from %s", len(instances), tc.region)
			}
		})
	}
}
