package recommend

import (
	"strings"

	"github.com/c3xdev/c3x/internal/domain"
)

// AWSRules returns the default set of AWS-targeted per-resource
// rules. Cross-resource rules (NAT consolidation, idle ALB) live in
// AWSTreeRules and are registered separately.
func AWSRules() []Rule {
	return []Rule{
		&Gp2ToGp3{},
		&EBSRightSize{},
		&EBSGp3OverprovisionedIOPS{},
		&IdleEIP{},
		&SingleAZDB{},
		&RDSStorageRightSize{},
		&LambdaMemoryRightSize{},
		&S3StorageClassTransition{},
		&CloudWatchLogRetention{},
		&InstanceFamilyDowngrade{},
		&RedshiftRightSize{},
		&NeptuneSingleNode{},
		&KinesisShardRightSize{},
	}
}

// AWSTreeRules returns AWS rules that need the whole resource tree.
func AWSTreeRules() []TreeRule {
	return []TreeRule{
		&NATGatewayConsolidation{},
		&IdleALB{},
	}
}

// =========================================================================
// Per-resource rules
// =========================================================================

// Gp2ToGp3 suggests migrating gp2 EBS volumes to gp3 — same capacity
// at ~20% lower per-GB rate and free IOPS up to 3,000.
type Gp2ToGp3 struct{}

func (Gp2ToGp3) Name() string { return "ebs.gp2-to-gp3" }

func (Gp2ToGp3) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_ebs_volume" {
		return nil
	}
	vt, ok := r.AttrString("type")
	if !ok || vt != "gp2" {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Migrate gp2 volume to gp3",
		Description: "gp3 charges ~20% less per GB than gp2 and includes 3,000 IOPS / 125 MB/s baseline " +
			"free. Drop-in replacement for most workloads.",
		AttributeChanges: map[string]any{"type": "gp3"},
	}}
}

// EBSRightSize flags volumes provisioned above 500 GB and suggests
// halving capacity. The engine verifies the proposed size produces
// savings before surfacing.
type EBSRightSize struct{}

func (EBSRightSize) Name() string { return "ebs.right-size" }

func (EBSRightSize) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_ebs_volume" {
		return nil
	}
	size, ok := r.AttrInt("size")
	if !ok || size < 500 {
		return nil
	}
	half := size / 2
	if half < 100 {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Consider halving the EBS volume size",
		Description: "Provisioned above 500 GB; volumes are commonly overprovisioned. Confirm actual " +
			"usage in CloudWatch and right-size before applying.",
		AttributeChanges: map[string]any{"size": half},
	}}
}

// EBSGp3OverprovisionedIOPS flags gp3 volumes with provisioned IOPS
// above the free-tier 3,000.
type EBSGp3OverprovisionedIOPS struct{}

func (EBSGp3OverprovisionedIOPS) Name() string { return "ebs.gp3-overprovisioned-iops" }

func (EBSGp3OverprovisionedIOPS) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_ebs_volume" {
		return nil
	}
	vt, _ := r.AttrString("type")
	if vt != "gp3" {
		return nil
	}
	iops, ok := r.AttrInt("iops")
	if !ok || iops <= 3000 {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Drop provisioned IOPS to gp3 baseline",
		Description: "gp3 includes 3,000 IOPS free; provisioning above only pays off when sustained " +
			"latency actually requires it. Audit VolumeQueueLength before applying.",
		AttributeChanges: map[string]any{"iops": int64(3000)},
	}}
}

// IdleEIP flags Elastic IPs and prompts an audit. Every EIP bills
// $0.005/hr since Feb 2024 regardless of attachment state.
type IdleEIP struct{}

func (IdleEIP) Name() string { return "eip.idle" }

func (IdleEIP) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_eip" {
		return nil
	}
	return []Proposal{{
		Category: "unused-resources",
		Title:    "Audit Elastic IP usage",
		Description: "Every Elastic IP bills $0.005/hr (≈$3.65/mo) regardless of attachment. If this " +
			"address isn't actively serving traffic, release it.",
		AttributeChanges: map[string]any{},
	}}
}

// SingleAZDB suggests dropping Multi-AZ to Single-AZ for non-prod
// databases. Multi-AZ doubles cost; for dev/staging the failover
// guarantee is usually unjustified.
type SingleAZDB struct{}

func (SingleAZDB) Name() string { return "rds.single-az-non-prod" }

func (SingleAZDB) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_db_instance" {
		return nil
	}
	multiAZ, _ := r.AttrBool("multi_az")
	if !multiAZ {
		return nil
	}
	id, _ := r.AttrString("identifier")
	if !looksLikeNonProd(id) && !looksLikeNonProd(r.Ref.Name) {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Drop Multi-AZ for non-production RDS",
		Description: "Multi-AZ roughly doubles instance cost. For dev/staging the failover guarantee " +
			"is usually unnecessary; reserve it for production.",
		AttributeChanges: map[string]any{"multi_az": false},
	}}
}

// RDSStorageRightSize flags RDS instances with > 200 GB allocated
// storage and suggests halving.
type RDSStorageRightSize struct{}

func (RDSStorageRightSize) Name() string { return "rds.storage-right-size" }

func (RDSStorageRightSize) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_db_instance" {
		return nil
	}
	size, ok := r.AttrInt("allocated_storage")
	if !ok || size < 200 {
		return nil
	}
	half := size / 2
	if half < 50 {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Reduce RDS allocated storage",
		Description: "Allocated above 200 GB. Confirm usage in CloudWatch FreeStorageSpace; RDS " +
			"storage is provisioned, not pay-per-use, so headroom translates directly to spend.",
		AttributeChanges: map[string]any{"allocated_storage": half},
	}}
}

// LambdaMemoryRightSize is a soft suggestion to drop very-high
// memory configurations.
type LambdaMemoryRightSize struct{}

func (LambdaMemoryRightSize) Name() string { return "lambda.memory-right-size" }

func (LambdaMemoryRightSize) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_lambda_function" {
		return nil
	}
	mem, ok := r.AttrInt("memory_size")
	if !ok || mem <= 1024 {
		return nil
	}
	suggested := int64(1024)
	if mem >= 4096 {
		suggested = mem / 2
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Right-size Lambda memory",
		Description: "Memory configuration drives both CPU and price. Most functions don't need > 1 GB; " +
			"tune with AWS Compute Optimizer or profile against duration metrics.",
		AttributeChanges: map[string]any{"memory_size": suggested},
	}}
}

// S3StorageClassTransition surfaces a lifecycle-policy prompt for
// buckets with > 1 TB of standard-class storage. The catalog doesn't
// yet model per-class differentiation, so this rule is advisory.
type S3StorageClassTransition struct{}

func (S3StorageClassTransition) Name() string { return "s3.storage-class" }

func (S3StorageClassTransition) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_s3_bucket" {
		return nil
	}
	gb, ok := r.AttrInt("standard_storage_gb")
	if !ok || gb < 1000 {
		return nil
	}
	return []Proposal{{
		Category: "lifecycle",
		Title:    "Add an S3 lifecycle policy for cold data",
		Description: "Buckets > 1 TB usually have a long tail of infrequently-accessed objects. A " +
			"lifecycle rule moving objects > 30 days old to Glacier IA can cut storage cost by 60-80%.",
		AttributeChanges: map[string]any{},
	}}
}

// CloudWatchLogRetention surfaces log groups without retention.
type CloudWatchLogRetention struct{}

func (CloudWatchLogRetention) Name() string { return "cloudwatch.log-retention" }

func (CloudWatchLogRetention) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_cloudwatch_log_group" {
		return nil
	}
	gb, ok := r.AttrInt("monthly_data_stored_gb")
	if !ok || gb < 10 {
		return nil
	}
	suggested := gb / 2
	return []Proposal{{
		Category: "lifecycle",
		Title:    "Set a CloudWatch Logs retention policy",
		Description: "Log groups without retention accumulate indefinitely. 30-day retention is a " +
			"reasonable default for ops logs; configure via `retention_in_days`.",
		AttributeChanges: map[string]any{"monthly_data_stored_gb": suggested},
	}}
}

// InstanceFamilyDowngrade suggests m5/m6i → t3 for non-prod
// instances.
type InstanceFamilyDowngrade struct{}

func (InstanceFamilyDowngrade) Name() string { return "ec2.family-downgrade-non-prod" }

func (InstanceFamilyDowngrade) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_instance" {
		return nil
	}
	if !looksLikeNonProd(r.Ref.Name) {
		return nil
	}
	it, ok := r.AttrString("instance_type")
	if !ok {
		return nil
	}
	if !strings.HasPrefix(it, "m5.") && !strings.HasPrefix(it, "m6i.") {
		return nil
	}
	suggested := mapToBurstable(it)
	if suggested == "" || suggested == it {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Downgrade to burstable t3 for non-production",
		Description: "Non-prod workloads rarely need m5's sustained performance. t3 burstable costs " +
			"~50% less at the same vCPU baseline. Monitor CPU credits if traffic is bursty.",
		AttributeChanges: map[string]any{"instance_type": suggested},
	}}
}

func mapToBurstable(m5size string) string {
	switch m5size {
	case "m5.large", "m6i.large":
		return "t3.medium"
	case "m5.xlarge", "m6i.xlarge":
		return "t3.large"
	case "m5.2xlarge", "m6i.2xlarge":
		return "t3.xlarge"
	case "m5.4xlarge", "m6i.4xlarge":
		return "t3.2xlarge"
	}
	return ""
}

// RedshiftRightSize flags multi-node Redshift clusters.
type RedshiftRightSize struct{}

func (RedshiftRightSize) Name() string { return "redshift.right-size" }

func (RedshiftRightSize) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_redshift_cluster" {
		return nil
	}
	n, ok := r.AttrInt("number_of_nodes")
	if !ok || n < 4 {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Audit Redshift node count",
		Description: "≥4 nodes is significant spend; profile against query concurrency before " +
			"committing. Concurrency Scaling absorbs burst workloads on a smaller base cluster.",
		AttributeChanges: map[string]any{"number_of_nodes": n - 1},
	}}
}

// NeptuneSingleNode suggests single-instance Neptune for non-prod.
type NeptuneSingleNode struct{}

func (NeptuneSingleNode) Name() string { return "neptune.single-instance-non-prod" }

func (NeptuneSingleNode) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_neptune_cluster" {
		return nil
	}
	if !looksLikeNonProd(r.Ref.Name) {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Use a single-instance Neptune cluster in non-prod",
		Description: "Default Neptune clusters provision a writer + reader; non-prod can drop the " +
			"reader. Configure `cluster_size = 1`.",
		AttributeChanges: map[string]any{"reader_count": int64(0)},
	}}
}

// KinesisShardRightSize flags wide Kinesis streams.
type KinesisShardRightSize struct{}

func (KinesisShardRightSize) Name() string { return "kinesis.shard-right-size" }

func (KinesisShardRightSize) Propose(r domain.Resource) []Proposal {
	if r.Ref.Kind != "aws_kinesis_stream" {
		return nil
	}
	shards, ok := r.AttrInt("shard_count")
	if !ok || shards < 4 {
		return nil
	}
	return []Proposal{{
		Category: "right-sizing",
		Title:    "Audit Kinesis shard count",
		Description: "Each shard bills hourly. On-demand mode auto-scales and may be cheaper if " +
			"traffic doesn't sustain the provisioned shard limit.",
		AttributeChanges: map[string]any{"shard_count": shards - 1},
	}}
}

// =========================================================================
// Tree rules — analyse the whole resource list before proposing
// =========================================================================

// NATGatewayConsolidation surfaces savings when an account runs
// > 1 NAT gateway. Multi-AZ deployments typically have one per AZ;
// non-prod can usually consolidate to a single AZ.
type NATGatewayConsolidation struct{}

func (NATGatewayConsolidation) Name() string { return "vpc.nat-consolidation" }

func (NATGatewayConsolidation) ProposeTree(resources []domain.Resource) []TreeProposal {
	var nats []domain.Resource
	for _, r := range resources {
		if r.Ref.Kind == "aws_nat_gateway" {
			nats = append(nats, r)
		}
	}
	if len(nats) < 2 {
		return nil
	}
	// Model the consolidation as a single multi-target proposal:
	//   - the surviving NAT (index 0) gains `monthly_inter_az_gb`
	//     equal to the sum of the removed NATs' data, so the new
	//     $0.02/GB cross-AZ line item shows up in its breakdown
	//   - every other NAT has its `monthly_data_processed_gb` and
	//     `monthly_hours` zeroed, removing it from billing
	// The engine compares the sum of baseline costs to the sum of
	// modified costs and reports the NET delta.
	survivor := nats[0]
	totalRemovedGB := int64(0)
	changes := map[domain.Reference]map[string]any{}
	for _, n := range nats[1:] {
		gb, _ := n.AttrInt("monthly_data_processed_gb")
		totalRemovedGB += int64(gb)
		changes[n.Ref] = map[string]any{
			"monthly_data_processed_gb": int64(0),
			"monthly_hours":             int64(0),
		}
	}
	survivorGB, _ := survivor.AttrInt("monthly_inter_az_gb")
	changes[survivor.Ref] = map[string]any{
		"monthly_inter_az_gb": int64(survivorGB) + totalRemovedGB,
	}
	return []TreeProposal{{
		PrimaryRef: survivor.Ref,
		Category:   "right-sizing",
		Title:      "Consolidate NAT gateways (net of inter-AZ transfer)",
		Description: "Multi-AZ NAT is a high-availability pattern; non-prod tolerates the reduced " +
			"redundancy. Net savings = removed NAT charges − $0.02/GB inter-AZ data transfer " +
			"the consolidation introduces on the surviving NAT.",
		Changes: changes,
	}}
}

// IdleALB flags load balancers when no target group sibling exists
// in the parsed configuration.
type IdleALB struct{}

func (IdleALB) Name() string { return "elb.idle" }

func (IdleALB) ProposeTree(resources []domain.Resource) []TreeProposal {
	// Index every aws_lb_listener's `load_balancer_arn` reference. The
	// HCL parser surfaces interpolations like
	// `aws_lb.api.arn` as the string `<ref:aws_lb.api>` (the parser's
	// placeholder when a referenced resource isn't fully resolvable);
	// it also passes through literal ARN strings unchanged. We accept
	// either shape — the substring match against the LB's Name field
	// is enough to recognise the reference without parsing the
	// placeholder syntax. False positives would have a listener whose
	// ARN string happens to contain another LB's exact name, which is
	// improbable in practice.
	listenerRefs := []string{}
	for _, r := range resources {
		if r.Ref.Kind != "aws_lb_listener" {
			continue
		}
		if s, ok := r.AttrString("load_balancer_arn"); ok {
			listenerRefs = append(listenerRefs, s)
		}
	}

	var props []TreeProposal
	for _, r := range resources {
		if r.Ref.Kind != "aws_lb" {
			continue
		}
		// Does any listener reference this LB by name? Check both the
		// `<ref:aws_lb.NAME>` placeholder and a substring containing
		// the LB's Name, so users authoring listeners via either
		// HCL interpolation or hand-written ARNs both get matched.
		matched := false
		needle := "aws_lb." + r.Ref.Name
		for _, ref := range listenerRefs {
			if strings.Contains(ref, needle) || strings.Contains(ref, "/"+r.Ref.Name+"/") {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		props = append(props, TreeProposal{
			PrimaryRef: r.Ref,
			Category:   "unused-resources",
			Title:      "Audit ELB usage — no listener references this LB",
			Description: "This ALB / NLB has no aws_lb_listener whose load_balancer_arn points at it. " +
				"Without a listener it cannot accept traffic; either delete or wire one up. The check " +
				"walks aws_lb_listener references so an LB still counts as used even when its target " +
				"groups live outside Terraform.",
			Changes: map[domain.Reference]map[string]any{
				r.Ref: {
					"monthly_lcu_hours": int64(0),
				},
			},
		})
	}
	return props
}

// =========================================================================
// Helpers
// =========================================================================

func looksLikeNonProd(s string) bool {
	for _, hint := range []string{"dev", "test", "staging", "stg", "qa", "sandbox"} {
		if containsCaseInsensitive(s, hint) {
			return true
		}
	}
	return false
}

func containsCaseInsensitive(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			a, b := haystack[i+j], needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
