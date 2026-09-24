package testutils

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	stypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
)

// MockSTSClient implements stscreds.AssumeRoleAPIClient for testing.
// Set ReturnErr to simulate AssumeRole failures.
var _ stscreds.AssumeRoleAPIClient = (*MockSTSClient)(nil)

type MockSTSClient struct {
	mu                 sync.Mutex
	wasCalled          bool
	CapturedExternalID string // set to the ExternalId value from the AssumeRole call, if any
	ReturnErr          error
}

func (m *MockSTSClient) AssumeRole(_ context.Context, params *sts.AssumeRoleInput, _ ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wasCalled = true
	if params.ExternalId != nil {
		m.CapturedExternalID = *params.ExternalId
	}
	if m.ReturnErr != nil {
		return nil, m.ReturnErr
	}
	return &sts.AssumeRoleOutput{
		Credentials: &stypes.Credentials{
			AccessKeyId:     aws.String("ASIAIOSFODNN7EXAMPLE"),
			SecretAccessKey: aws.String("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
			SessionToken:    aws.String("session-token"),
			Expiration:      aws.Time(time.Now().Add(time.Hour)),
		},
	}, nil
}

func (m *MockSTSClient) Called() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.wasCalled
}
