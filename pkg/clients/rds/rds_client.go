package rds

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type RDSClient struct {
	client *rds.Client
}

// AWS Relational Database Service (RDS) manages relational databases in the cloud.
// This client focuses on discovery instances for comprehensive database performance monitoring.

// RDSClient wraps the AWS RDS SDK with application-specific database discovery functionality.
// It provides methods for describing database instances.
func NewRDSClient(region string) (*RDSClient, error) {
	return NewRDSClientWithEndpoint(region, "")
}

// NewRDSClientWithRole creates an RDS client that assumes the given IAM role before
// calling the RDS API. externalID is optional (pass "" to omit). If roleARN is empty
// it behaves identically to NewRDSClient.
func NewRDSClientWithRole(region, roleARN, externalID string) (*RDSClient, error) {
	if roleARN == "" {
		return NewRDSClient(region)
	}

	log.Printf("[RDS] Creating new RDS client with assumed role: %s", roleARN)
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		log.Printf("[RDS] ERROR: Failed to load AWS config: %v", err)
		return nil, err
	}

	stsClient := sts.NewFromConfig(cfg)
	client, _ := newRDSClientWithSTSClient(cfg, roleARN, externalID, stsClient) // credCache only needed in tests
	log.Printf("[RDS] STS credential provider attached for role assumption, region: %s", region)
	return client, nil
}

// newRDSClientWithSTSClient intentionally mirrors newPIClientWithSTSClient in pi_client.go.
// Keep both in sync when changing AssumeRoleOptions.
func newRDSClientWithSTSClient(cfg aws.Config, roleARN, externalID string, stsClient stscreds.AssumeRoleAPIClient) (*RDSClient, *aws.CredentialsCache) {
	creds := stscreds.NewAssumeRoleProvider(stsClient, roleARN, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = "rds-pi-exporter/rds"
		if externalID != "" {
			o.ExternalID = aws.String(externalID)
		}
	})
	credCache := aws.NewCredentialsCache(creds)
	cfg.Credentials = credCache
	return newRDSClientFromConfig(cfg, ""), credCache
}

func NewRDSClientWithEndpoint(region, endpoint string) (*RDSClient, error) {
	log.Println("[RDS] Creating new RDS client...")
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		log.Printf("[RDS] FATAL: Failed to load AWS config: %v", err)
		return nil, err
	}

	log.Printf("[RDS] AWS config loaded, region: %s", cfg.Region)
	return newRDSClientFromConfig(cfg, endpoint), nil
}

func newRDSClientFromConfig(cfg aws.Config, endpoint string) *RDSClient {
	client := rds.NewFromConfig(cfg)
	if endpoint != "" {
		client = rds.NewFromConfig(cfg, func(o *rds.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
		log.Printf("[RDS] Using custom endpoint: %s", endpoint)
	}
	return &RDSClient{client: client}
}

func (rdsClient *RDSClient) DescribeDBInstancesPaginator(ctx context.Context) ([]types.DBInstance, error) {
	input := &rds.DescribeDBInstancesInput{
		MaxRecords: aws.Int32(100),
	}

	var allInstances []types.DBInstance

	paginator := rds.NewDescribeDBInstancesPaginator(rdsClient.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Printf("[RDS] Failed to describe DB instances: %v", err)
			return nil, err
		}

		allInstances = append(allInstances, page.DBInstances...)
	}

	log.Printf("[RDS] Retrieved %d DB instances", len(allInstances))
	return allInstances, nil
}
