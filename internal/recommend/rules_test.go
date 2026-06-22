package recommend_test

// Per-rule trigger / silent tests. Each test fixes the rule's
// contract: which (Kind, attributes) combination produces a proposal
// and which does not. The tests don't go through the calculator —
// they only verify the rule emits a non-empty []Proposal under the
// trigger condition. Engine-level savings math is covered separately
// by the round-trip tests in recommend_test.go.

import (
	"testing"

	"github.com/c3xdev/c3x/internal/domain"
	"github.com/c3xdev/c3x/internal/recommend"
)

type proposer interface {
	Propose(r domain.Resource) []recommend.Proposal
}

type ruleCase struct {
	name      string
	rule      proposer
	resource  domain.Resource
	wantCount int
}

func res(kind, name string, attrs map[string]any) domain.Resource {
	return domain.Resource{
		Ref:        domain.Reference{Kind: kind, Name: name},
		Attributes: attrs,
	}
}

func TestPerResourceRulesTriggerConditions(t *testing.T) {
	t.Parallel()

	cases := []ruleCase{
		// =====================================================
		// AWS — EBS family
		// =====================================================
		{
			name:      "Gp2ToGp3 triggers on gp2 volume",
			rule:      &recommend.Gp2ToGp3{},
			resource:  res("aws_ebs_volume", "x", map[string]any{"type": "gp2", "size": float64(100)}),
			wantCount: 1,
		},
		{
			name:      "Gp2ToGp3 silent on gp3 volume",
			rule:      &recommend.Gp2ToGp3{},
			resource:  res("aws_ebs_volume", "x", map[string]any{"type": "gp3"}),
			wantCount: 0,
		},
		{
			name:      "Gp2ToGp3 silent on wrong kind",
			rule:      &recommend.Gp2ToGp3{},
			resource:  res("aws_instance", "x", map[string]any{"type": "gp2"}),
			wantCount: 0,
		},
		{
			name:      "EBSRightSize triggers on oversized gp3",
			rule:      &recommend.EBSRightSize{},
			resource:  res("aws_ebs_volume", "x", map[string]any{"type": "gp3", "size": float64(2000)}),
			wantCount: 1,
		},
		{
			name:      "EBSRightSize silent on small disk",
			rule:      &recommend.EBSRightSize{},
			resource:  res("aws_ebs_volume", "x", map[string]any{"type": "gp3", "size": float64(50)}),
			wantCount: 0,
		},
		{
			name:      "EBSGp3OverprovisionedIOPS triggers above 3000",
			rule:      &recommend.EBSGp3OverprovisionedIOPS{},
			resource:  res("aws_ebs_volume", "x", map[string]any{"type": "gp3", "iops": float64(8000)}),
			wantCount: 1,
		},
		{
			name:      "EBSGp3OverprovisionedIOPS silent at default 3000",
			rule:      &recommend.EBSGp3OverprovisionedIOPS{},
			resource:  res("aws_ebs_volume", "x", map[string]any{"type": "gp3", "iops": float64(3000)}),
			wantCount: 0,
		},

		// =====================================================
		// AWS — EIP / EC2 / family downgrade
		// =====================================================
		{
			name:      "IdleEIP triggers on unattached EIP",
			rule:      &recommend.IdleEIP{},
			resource:  res("aws_eip", "x", map[string]any{}),
			wantCount: 1,
		},
		{
			name:      "IdleEIP silent on wrong kind",
			rule:      &recommend.IdleEIP{},
			resource:  res("aws_instance", "x", map[string]any{}),
			wantCount: 0,
		},
		{
			name:      "InstanceFamilyDowngrade triggers on dev m5 instance",
			rule:      &recommend.InstanceFamilyDowngrade{},
			resource:  res("aws_instance", "dev-api", map[string]any{"instance_type": "m5.xlarge"}),
			wantCount: 1,
		},
		{
			name:      "InstanceFamilyDowngrade silent on prod-named instance",
			rule:      &recommend.InstanceFamilyDowngrade{},
			resource:  res("aws_instance", "prod-api", map[string]any{"instance_type": "m5.xlarge"}),
			wantCount: 0,
		},

		// =====================================================
		// AWS — RDS / Redshift / Neptune
		// =====================================================
		{
			name:      "SingleAZDB triggers on dev Multi-AZ db",
			rule:      &recommend.SingleAZDB{},
			resource:  res("aws_db_instance", "dev-orders", map[string]any{"multi_az": true}),
			wantCount: 1,
		},
		{
			name:      "SingleAZDB silent on prod Multi-AZ db",
			rule:      &recommend.SingleAZDB{},
			resource:  res("aws_db_instance", "prod-orders", map[string]any{"multi_az": true}),
			wantCount: 0,
		},
		{
			name:      "RDSStorageRightSize triggers on oversized storage",
			rule:      &recommend.RDSStorageRightSize{},
			resource:  res("aws_db_instance", "x", map[string]any{"allocated_storage": float64(2000)}),
			wantCount: 1,
		},
		{
			name:      "RDSStorageRightSize silent on small storage",
			rule:      &recommend.RDSStorageRightSize{},
			resource:  res("aws_db_instance", "x", map[string]any{"allocated_storage": float64(50)}),
			wantCount: 0,
		},
		{
			name:      "RedshiftRightSize triggers above 4 nodes",
			rule:      &recommend.RedshiftRightSize{},
			resource:  res("aws_redshift_cluster", "warehouse", map[string]any{"node_type": "ra3.4xlarge", "number_of_nodes": int64(6)}),
			wantCount: 1,
		},
		{
			name:      "RedshiftRightSize silent at 2 nodes",
			rule:      &recommend.RedshiftRightSize{},
			resource:  res("aws_redshift_cluster", "warehouse", map[string]any{"node_type": "ra3.4xlarge", "number_of_nodes": int64(2)}),
			wantCount: 0,
		},
		{
			name:      "NeptuneSingleNode triggers on dev cluster with replicas",
			rule:      &recommend.NeptuneSingleNode{},
			resource:  res("aws_neptune_cluster", "dev-graph", map[string]any{"replica_count": float64(2)}),
			wantCount: 1,
		},
		{
			name:      "NeptuneSingleNode silent on prod cluster",
			rule:      &recommend.NeptuneSingleNode{},
			resource:  res("aws_neptune_cluster", "prod-graph", map[string]any{"replica_count": float64(2)}),
			wantCount: 0,
		},

		// =====================================================
		// AWS — Lambda / S3 / CloudWatch / Kinesis
		// =====================================================
		{
			name:      "LambdaMemoryRightSize triggers on 4GB Lambda",
			rule:      &recommend.LambdaMemoryRightSize{},
			resource:  res("aws_lambda_function", "x", map[string]any{"memory_size": float64(4096)}),
			wantCount: 1,
		},
		{
			name:      "LambdaMemoryRightSize silent on 256MB Lambda",
			rule:      &recommend.LambdaMemoryRightSize{},
			resource:  res("aws_lambda_function", "x", map[string]any{"memory_size": float64(256)}),
			wantCount: 0,
		},
		{
			name:      "S3StorageClassTransition triggers above 1TB",
			rule:      &recommend.S3StorageClassTransition{},
			resource:  res("aws_s3_bucket", "x", map[string]any{"standard_storage_gb": int64(2000)}),
			wantCount: 1,
		},
		{
			name:      "S3StorageClassTransition silent on small bucket",
			rule:      &recommend.S3StorageClassTransition{},
			resource:  res("aws_s3_bucket", "x", map[string]any{"standard_storage_gb": int64(10)}),
			wantCount: 0,
		},
		{
			name:      "CloudWatchLogRetention triggers above 10GB stored",
			rule:      &recommend.CloudWatchLogRetention{},
			resource:  res("aws_cloudwatch_log_group", "x", map[string]any{"monthly_data_stored_gb": int64(100)}),
			wantCount: 1,
		},
		{
			name:      "CloudWatchLogRetention silent on small log group",
			rule:      &recommend.CloudWatchLogRetention{},
			resource:  res("aws_cloudwatch_log_group", "x", map[string]any{"monthly_data_stored_gb": int64(1)}),
			wantCount: 0,
		},
		{
			name:      "KinesisShardRightSize triggers above 4 shards",
			rule:      &recommend.KinesisShardRightSize{},
			resource:  res("aws_kinesis_stream", "x", map[string]any{"shard_count": float64(8)}),
			wantCount: 1,
		},
		{
			name:      "KinesisShardRightSize silent at 1 shard",
			rule:      &recommend.KinesisShardRightSize{},
			resource:  res("aws_kinesis_stream", "x", map[string]any{"shard_count": float64(1)}),
			wantCount: 0,
		},

		// =====================================================
		// Azure
		// =====================================================
		{
			name:      "AzureBurstableForDev triggers on dev D-series VM",
			rule:      &recommend.AzureBurstableForDev{},
			resource:  res("azurerm_linux_virtual_machine", "dev-api", map[string]any{"size": "Standard_D2s_v5"}),
			wantCount: 1,
		},
		{
			name:      "AzureBurstableForDev silent on prod D-series VM",
			rule:      &recommend.AzureBurstableForDev{},
			resource:  res("azurerm_linux_virtual_machine", "prod-api", map[string]any{"size": "Standard_D2s_v5"}),
			wantCount: 0,
		},
		{
			name:      "AzureStorageCoolTier triggers above 500GB",
			rule:      &recommend.AzureStorageCoolTier{},
			resource:  res("azurerm_storage_account", "x", map[string]any{"monthly_storage_gb": float64(1000)}),
			wantCount: 1,
		},
		{
			name:      "AzureStorageCoolTier silent below 500GB",
			rule:      &recommend.AzureStorageCoolTier{},
			resource:  res("azurerm_storage_account", "x", map[string]any{"monthly_storage_gb": float64(100)}),
			wantCount: 0,
		},
		{
			name:      "AzurePostgresB1Right triggers on dev GP server",
			rule:      &recommend.AzurePostgresB1Right{},
			resource:  res("azurerm_postgresql_flexible_server", "dev-db", map[string]any{"sku_name": "GP_Standard_D2s_v3"}),
			wantCount: 1,
		},
		{
			name:      "AzurePostgresB1Right silent on already-Burstable SKU",
			rule:      &recommend.AzurePostgresB1Right{},
			resource:  res("azurerm_postgresql_flexible_server", "dev-db", map[string]any{"sku_name": "B_Standard_B1ms"}),
			wantCount: 0,
		},
		{
			name:      "AzureLogAnalyticsRetention triggers above 100GB",
			rule:      &recommend.AzureLogAnalyticsRetention{},
			resource:  res("azurerm_log_analytics_workspace", "x", map[string]any{"monthly_retained_gb": float64(500)}),
			wantCount: 1,
		},
		{
			name:      "AzureLogAnalyticsRetention silent below 100GB",
			rule:      &recommend.AzureLogAnalyticsRetention{},
			resource:  res("azurerm_log_analytics_workspace", "x", map[string]any{"monthly_retained_gb": float64(10)}),
			wantCount: 0,
		},
		{
			name:      "AzureRedisBasicForDev triggers on dev Standard SKU",
			rule:      &recommend.AzureRedisBasicForDev{},
			resource:  res("azurerm_redis_cache", "dev-cache", map[string]any{"sku_name": "Standard"}),
			wantCount: 1,
		},
		{
			name:      "AzureRedisBasicForDev silent on already-Basic SKU",
			rule:      &recommend.AzureRedisBasicForDev{},
			resource:  res("azurerm_redis_cache", "dev-cache", map[string]any{"sku_name": "Basic"}),
			wantCount: 0,
		},
		{
			name:      "AzureUnattachedDisk triggers on >100GB managed disk",
			rule:      &recommend.AzureUnattachedDisk{},
			resource:  res("azurerm_managed_disk", "x", map[string]any{"disk_size_gb": float64(500)}),
			wantCount: 1,
		},
		{
			name:      "AzureUnattachedDisk silent on small disk",
			rule:      &recommend.AzureUnattachedDisk{},
			resource:  res("azurerm_managed_disk", "x", map[string]any{"disk_size_gb": float64(20)}),
			wantCount: 0,
		},

		// =====================================================
		// GCP
		// =====================================================
		{
			name:      "GCPPDStandardToBalanced triggers on pd-standard",
			rule:      &recommend.GCPPDStandardToBalanced{},
			resource:  res("google_compute_disk", "x", map[string]any{"type": "pd-standard", "size": float64(200)}),
			wantCount: 1,
		},
		{
			name:      "GCPPDStandardToBalanced silent on pd-balanced",
			rule:      &recommend.GCPPDStandardToBalanced{},
			resource:  res("google_compute_disk", "x", map[string]any{"type": "pd-balanced"}),
			wantCount: 0,
		},
		{
			name:      "GCPCloudSQLNonProdTier triggers on dev Enterprise tier",
			rule:      &recommend.GCPCloudSQLNonProdTier{},
			resource:  res("google_sql_database_instance", "dev-db", map[string]any{"tier": "db-custom-4-15360"}),
			wantCount: 1,
		},
		{
			name:      "GCPCloudSQLNonProdTier silent on prod tier",
			rule:      &recommend.GCPCloudSQLNonProdTier{},
			resource:  res("google_sql_database_instance", "prod-db", map[string]any{"tier": "db-custom-4-15360"}),
			wantCount: 0,
		},
		{
			name:      "GCPStorageBucketColdline triggers on large bucket",
			rule:      &recommend.GCPStorageBucketColdline{},
			resource:  res("google_storage_bucket", "x", map[string]any{"storage_class": "STANDARD", "monthly_storage_gb": float64(1000)}),
			wantCount: 1,
		},
		{
			name:      "GCPStorageBucketColdline silent on small bucket",
			rule:      &recommend.GCPStorageBucketColdline{},
			resource:  res("google_storage_bucket", "x", map[string]any{"storage_class": "STANDARD", "monthly_storage_gb": float64(10)}),
			wantCount: 0,
		},
		{
			name:      "GCPUnusedStaticIP triggers on unattached IP",
			rule:      &recommend.GCPUnusedStaticIP{},
			resource:  res("google_compute_address", "x", map[string]any{}),
			wantCount: 1,
		},
		{
			name:      "GCPE2InsteadOfN1 triggers on any n1 machine",
			rule:      &recommend.GCPE2InsteadOfN1{},
			resource:  res("google_compute_instance", "api", map[string]any{"machine_type": "n1-standard-4"}),
			wantCount: 1,
		},
		{
			name:      "GCPE2InsteadOfN1 silent on e2 machine",
			rule:      &recommend.GCPE2InsteadOfN1{},
			resource:  res("google_compute_instance", "api", map[string]any{"machine_type": "e2-standard-2"}),
			wantCount: 0,
		},
		{
			name:      "GCPLoggingRetention triggers above 50GiB ingestion",
			rule:      &recommend.GCPLoggingRetention{},
			resource:  res("google_logging_project_bucket", "x", map[string]any{"monthly_ingestion_gib": int64(200)}),
			wantCount: 1,
		},
		{
			name:      "GCPLoggingRetention silent below 50GiB",
			rule:      &recommend.GCPLoggingRetention{},
			resource:  res("google_logging_project_bucket", "x", map[string]any{"monthly_ingestion_gib": int64(1)}),
			wantCount: 0,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.rule.Propose(tc.resource)
			if len(got) != tc.wantCount {
				t.Errorf("Propose returned %d proposals, want %d (rule=%T, ref=%v)",
					len(got), tc.wantCount, tc.rule, tc.resource.Ref)
			}
		})
	}
}
