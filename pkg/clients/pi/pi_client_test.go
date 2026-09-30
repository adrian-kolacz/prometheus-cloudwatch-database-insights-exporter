package pi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"

	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/awslabs/prometheus-cloudwatch-database-insights-exporter/pkg/testutils"
)

// newPIClientWithSTSClient is a test-only helper that injects a mock STS client
// so unit tests can verify the AssumeRole credential chain without network calls.
func newPIClientWithSTSClient(cfg aws.Config, roleARN, externalID string, stsClient stscreds.AssumeRoleAPIClient) (*PIClient, *aws.CredentialsCache) {
	credCache := utils.NewAssumeRoleCredCache(stsClient, roleARN, externalID, "rds-pi-exporter-pi")
	cfg.Credentials = credCache
	return newPIClientFromConfig(cfg, ""), credCache
}

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
		piClient, err := NewPIClientWithRole(testutils.TestRegion, "", "")
		assert.NoError(t, err)
		assert.NotNil(t, piClient)
		assert.NotNil(t, piClient.client)
	})

	t.Run("non-empty roleARN constructs client without network calls", func(t *testing.T) {
		// AssumeRole is lazy — STS is not called until the first API request,
		// so construction succeeds even without reachable AWS credentials.
		piClient, err := NewPIClientWithRole(testutils.TestRegion, "arn:aws:iam::123456789012:role/TestRole", "")
		assert.NoError(t, err)
		assert.NotNil(t, piClient)
		assert.NotNil(t, piClient.client)
	})
}

func TestNewPIClientWithSTSClient(t *testing.T) {
	newCfg := func(t *testing.T) aws.Config {
		t.Helper()
		cfg, err := config.LoadDefaultConfig(context.TODO(),
			config.WithRegion(testutils.TestRegion),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "TOKEN")),
		)
		require.NoError(t, err)
		return cfg
	}

	t.Run("AssumeRole is called on first credential retrieval and returns expected credentials", func(t *testing.T) {
		mockSTS := &testutils.MockSTSClient{}

		piClient, credCache := newPIClientWithSTSClient(newCfg(t), "arn:aws:iam::123456789012:role/TestRole", "", mockSTS)
		require.NotNil(t, piClient)
		require.NotNil(t, piClient.client)

		assert.False(t, mockSTS.Called(), "STS should not be called before credential retrieval")

		creds, err := credCache.Retrieve(context.TODO())
		require.NoError(t, err)

		assert.True(t, mockSTS.Called(), "STS AssumeRole should have been called on credential retrieval")
		assert.Equal(t, "TEST-ACCESS-KEY-ID-0", creds.AccessKeyID)
		assert.Equal(t, "session-token", creds.SessionToken)
		assert.Equal(t, "arn:aws:iam::123456789012:role/TestRole", mockSTS.CapturedRoleArnValue())
		assert.Equal(t, "rds-pi-exporter-pi", mockSTS.CapturedRoleSessionNameValue())
	})

	t.Run("ExternalID is forwarded to AssumeRole when set", func(t *testing.T) {
		mockSTS := &testutils.MockSTSClient{}

		_, credCache := newPIClientWithSTSClient(newCfg(t), "arn:aws:iam::123456789012:role/TestRole", "my-external-id", mockSTS)

		_, err := credCache.Retrieve(context.TODO())
		require.NoError(t, err)
		assert.Equal(t, "my-external-id", mockSTS.CapturedExternalIDValue())
	})

	t.Run("ExternalID is not sent when empty", func(t *testing.T) {
		mockSTS := &testutils.MockSTSClient{}

		_, credCache := newPIClientWithSTSClient(newCfg(t), "arn:aws:iam::123456789012:role/TestRole", "", mockSTS)

		_, err := credCache.Retrieve(context.TODO())
		require.NoError(t, err)
		assert.Empty(t, mockSTS.CapturedExternalIDValue())
	})

	t.Run("AssumeRole error is propagated through credential retrieval", func(t *testing.T) {
		mockSTS := &testutils.MockSTSClient{ReturnErr: fmt.Errorf("AccessDenied: not authorized to assume role")}

		_, credCache := newPIClientWithSTSClient(newCfg(t), "arn:aws:iam::123456789012:role/TestRole", "", mockSTS)

		_, err := credCache.Retrieve(context.TODO())
		assert.Error(t, err)
		assert.True(t, mockSTS.Called())
		assert.Contains(t, err.Error(), "AccessDenied")
	})
}

func TestNewPIClientWithRoleEndToEnd(t *testing.T) {
	var (
		mu           sync.Mutex
		stsCallCount int
		piAuthHeader string
	)

	const stsXML = `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>TEST-ACCESS-KEY-ID-0</AccessKeyId><SecretAccessKey>TEST-SECRET-KEY-NOT-REAL-000000000000</SecretAccessKey><SessionToken>assumed-session-token</SessionToken><Expiration>2030-01-01T00:00:00Z</Expiration></Credentials><AssumedRoleUser><Arn>arn:aws:sts::123456789012:assumed-role/TestRole/rds-pi-exporter/pi</Arn><AssumedRoleId>AROATEST:rds-pi-exporter/pi</AssumedRoleId></AssumedRoleUser></AssumeRoleResult><ResponseMetadata><RequestId>test</RequestId></ResponseMetadata></AssumeRoleResponse>`
	const piJSON = `{"Metrics":[]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-amz-json") {
			auth := r.Header.Get("Authorization")
			mu.Lock()
			piAuthHeader = auth
			mu.Unlock()
			w.Header().Set("Content-Type", "application/x-amz-json-1.1")
			fmt.Fprint(w, piJSON)
		} else {
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "AssumeRole") {
				mu.Lock()
				stsCallCount++
				mu.Unlock()
				w.Header().Set("Content-Type", "text/xml")
				fmt.Fprint(w, stsXML)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "unexpected request: method=%s path=%s body=%s", r.Method, r.URL.Path, string(body))
			}
		}
	}))
	defer server.Close()

	t.Setenv("AWS_ENDPOINT_URL", server.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDBASE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRETBASE")
	t.Setenv("AWS_SESSION_TOKEN", "")

	piClient, err := NewPIClientWithRole(testutils.TestRegion, "arn:aws:iam::123456789012:role/TestRole", "")
	require.NoError(t, err)

	_, err = piClient.ListAvailableResourceMetrics(context.Background(), "db-TESTRESOURCEID")
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, stsCallCount, "STS AssumeRole should have been called exactly once")
	assert.Contains(t, piAuthHeader, "TEST-ACCESS-KEY-ID-0", "PI request should use assumed role credentials, not base credentials")
}

func TestNewPIClientWithRoleAndExternalIDEndToEnd(t *testing.T) {
	var (
		mu             sync.Mutex
		stsRequestBody string
	)

	const stsXML = `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>TEST-ACCESS-KEY-ID-0</AccessKeyId><SecretAccessKey>TEST-SECRET-KEY-NOT-REAL-000000000000</SecretAccessKey><SessionToken>assumed-session-token</SessionToken><Expiration>2030-01-01T00:00:00Z</Expiration></Credentials><AssumedRoleUser><Arn>arn:aws:sts::123456789012:assumed-role/TestRole/rds-pi-exporter-pi</Arn><AssumedRoleId>AROATEST:rds-pi-exporter-pi</AssumedRoleId></AssumedRoleUser></AssumeRoleResult><ResponseMetadata><RequestId>test</RequestId></ResponseMetadata></AssumeRoleResponse>`
	const piJSON = `{"Metrics":[]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-amz-json") {
			w.Header().Set("Content-Type", "application/x-amz-json-1.1")
			fmt.Fprint(w, piJSON)
		} else {
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "AssumeRole") {
				mu.Lock()
				stsRequestBody = string(body)
				mu.Unlock()
				w.Header().Set("Content-Type", "text/xml")
				fmt.Fprint(w, stsXML)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "unexpected request: method=%s path=%s body=%s", r.Method, r.URL.Path, string(body))
			}
		}
	}))
	defer server.Close()

	t.Setenv("AWS_ENDPOINT_URL", server.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDBASE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRETBASE")
	t.Setenv("AWS_SESSION_TOKEN", "")

	piClient, err := NewPIClientWithRole(testutils.TestRegion, "arn:aws:iam::123456789012:role/TestRole", "my-external-id")
	require.NoError(t, err)

	_, err = piClient.ListAvailableResourceMetrics(context.Background(), "db-TESTRESOURCEID")
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	// STS uses application/x-www-form-urlencoded; special chars would be percent-encoded.
	// "my-external-id" contains only URL-safe chars so no encoding is needed here.
	assert.Contains(t, stsRequestBody, "ExternalId=my-external-id", "STS AssumeRole request should include ExternalId on the wire")
}
