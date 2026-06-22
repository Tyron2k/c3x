package cloudformation_test

import (
	"testing"

	"github.com/c3xdev/c3x/internal/parser/cloudformation"
)

func TestParsesSimpleYAMLTemplate(t *testing.T) {
	t.Parallel()
	yaml := `
AWSTemplateFormatVersion: '2010-09-09'
Resources:
  Web:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: m5.xlarge
      ImageId: ami-123
`
	got, err := cloudformation.ParseBytes([]byte(yaml), "test.yaml", cloudformation.Options{Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(got))
	}
	r := got[0]
	if r.Ref.Kind != "aws_instance" || r.Ref.Name != "Web" {
		t.Errorf("ref = %v, want aws_instance.Web", r.Ref)
	}
	if r.Attributes["instance_type"] != "m5.xlarge" {
		t.Errorf("instance_type = %v", r.Attributes["instance_type"])
	}
	if r.Attributes["ami"] != "ami-123" {
		t.Errorf("ami = %v", r.Attributes["ami"])
	}
	if r.Attributes["operating_system"] != "Linux" {
		t.Errorf("operating_system not stamped: %v", r.Attributes["operating_system"])
	}
	if r.Region == nil || *r.Region != "us-east-1" {
		t.Errorf("region = %v", r.Region)
	}
}

func TestParsesJSONTemplate(t *testing.T) {
	t.Parallel()
	doc := `{
		"Resources": {
			"DB": {
				"Type": "AWS::RDS::DBInstance",
				"Properties": {
					"DBInstanceClass": "db.t3.medium",
					"Engine": "postgres",
					"AllocatedStorage": 100,
					"StorageType": "gp3",
					"MultiAZ": false
				}
			}
		}
	}`
	got, err := cloudformation.ParseBytes([]byte(doc), "test.json", cloudformation.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Ref.Kind != "aws_db_instance" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Attributes["instance_class"] != "db.t3.medium" {
		t.Errorf("instance_class = %v", got[0].Attributes["instance_class"])
	}
	if got[0].Attributes["engine"] != "postgres" {
		t.Errorf("engine = %v", got[0].Attributes["engine"])
	}
}

func TestResolvesParameterRef(t *testing.T) {
	t.Parallel()
	yaml := `
Parameters:
  Size:
    Type: String
    Default: t3.small
Resources:
  Web:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: !Ref Size
      ImageId: ami-x
`
	got, _ := cloudformation.ParseBytes([]byte(yaml), "test.yaml", cloudformation.Options{})
	if got[0].Attributes["instance_type"] != "t3.small" {
		t.Errorf("Ref to parameter default didn't resolve: %v", got[0].Attributes["instance_type"])
	}
}

func TestParameterOverrideBeatsDefault(t *testing.T) {
	t.Parallel()
	yaml := `
Parameters:
  Size:
    Type: String
    Default: t3.small
Resources:
  Web:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: !Ref Size
      ImageId: ami-x
`
	got, _ := cloudformation.ParseBytes([]byte(yaml), "test.yaml", cloudformation.Options{
		Parameters: map[string]any{"Size": "m5.large"},
	})
	if got[0].Attributes["instance_type"] != "m5.large" {
		t.Errorf("override didn't win: %v", got[0].Attributes["instance_type"])
	}
}

func TestResolvesFnSub(t *testing.T) {
	t.Parallel()
	yaml := `
Parameters:
  Env:
    Type: String
    Default: prod
Resources:
  Bucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: !Sub "${Env}-data-bucket"
`
	got, _ := cloudformation.ParseBytes([]byte(yaml), "test.yaml", cloudformation.Options{})
	if got[0].Attributes["bucket"] != "prod-data-bucket" {
		t.Errorf("Fn::Sub didn't resolve: %v", got[0].Attributes["bucket"])
	}
}

func TestResolvesFnJoin(t *testing.T) {
	t.Parallel()
	yaml := `
Resources:
  Bucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: !Join ["-", ["my", "data", "bucket"]]
`
	got, _ := cloudformation.ParseBytes([]byte(yaml), "test.yaml", cloudformation.Options{})
	if got[0].Attributes["bucket"] != "my-data-bucket" {
		t.Errorf("Fn::Join didn't resolve: %v", got[0].Attributes["bucket"])
	}
}

func TestResolvesFindInMap(t *testing.T) {
	t.Parallel()
	yaml := `
Mappings:
  Sizes:
    prod:
      Instance: m5.xlarge
    dev:
      Instance: t3.small
Parameters:
  Env:
    Type: String
    Default: prod
Resources:
  Web:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: !FindInMap [Sizes, !Ref Env, Instance]
      ImageId: ami-x
`
	got, _ := cloudformation.ParseBytes([]byte(yaml), "test.yaml", cloudformation.Options{})
	if got[0].Attributes["instance_type"] != "m5.xlarge" {
		t.Errorf("FindInMap didn't resolve: %v", got[0].Attributes["instance_type"])
	}
}

func TestUnknownResourceTypeIsSkippedNotErrored(t *testing.T) {
	t.Parallel()
	yaml := `
Resources:
  Web:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: t3.small
      ImageId: ami-x
  Custom:
    Type: AWS::NotARealService::ThingThatDoesntExist
    Properties:
      Foo: bar
`
	got, err := cloudformation.ParseBytes([]byte(yaml), "test.yaml", cloudformation.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 resource (custom skipped), got %d", len(got))
	}
}

func TestFlattensOpenSearchClusterConfig(t *testing.T) {
	t.Parallel()
	yaml := `
Resources:
  Search:
    Type: AWS::OpenSearchService::Domain
    Properties:
      ClusterConfig:
        InstanceType: t3.small.search
        InstanceCount: 2
      EBSOptions:
        EBSEnabled: true
        VolumeType: gp3
        VolumeSize: 20
`
	got, _ := cloudformation.ParseBytes([]byte(yaml), "test.yaml", cloudformation.Options{})
	if len(got) != 1 {
		t.Fatalf("got %d resources", len(got))
	}
	if got[0].Attributes["cluster_config_instance_type"] != "t3.small.search" {
		t.Errorf("nested flatten failed: %+v", got[0].Attributes)
	}
	if got[0].Attributes["ebs_options_ebs_enabled"] != true {
		t.Errorf("nested flatten boolean failed: %+v", got[0].Attributes)
	}
}

func TestRefToUnknownIsPlaceholdered(t *testing.T) {
	t.Parallel()
	yaml := `
Resources:
  Web:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: t3.small
      ImageId: !Ref AnotherResource
`
	got, _ := cloudformation.ParseBytes([]byte(yaml), "test.yaml", cloudformation.Options{})
	ami, _ := got[0].Attributes["ami"].(string)
	if ami != "<ref:AnotherResource>" {
		t.Errorf("expected placeholder, got %q", ami)
	}
}
