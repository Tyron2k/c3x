package cloudformation_test

// Fuzz the CloudFormation parser. Two paths to stress:
//   1. The YAML AST short-form tag expansion (rewrap / expandIntrinsicTags)
//      — handling raw `!Ref`, `!Sub`, `!GetAtt` with adversarial inputs.
//   2. The Fn::Sub / Fn::Join / Fn::FindInMap evaluation chain — these
//      perform string interpolation and can OOM on pathological inputs
//      if recursion isn't bounded.
//
// Run with `go test ./internal/parser/cloudformation/... -fuzz=FuzzParseBytes -fuzztime=10s`.

import (
	"testing"

	"github.com/c3xdev/c3x/internal/parser/cloudformation"
)

func FuzzParseBytes(f *testing.F) {
	seeds := []string{
		`{}`,
		`{"Resources":{}}`,
		`Resources:
  Bucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: my-bucket
`,
		`Resources:
  Web:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: !Ref Size
`,
		// Nested intrinsics
		`Resources:
  X:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: !Sub "${!Ref Env}-${AWS::Region}"
`,
		// Unknown tags
		`Resources:
  X:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: !UnknownTag value
`,
		// Empty / pathological
		``,
		`null`,
		`Resources: null`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		// Bounded input — fuzzer occasionally produces 1MB+ that
		// makes the run noisy without finding real bugs.
		if len(data) > 100_000 {
			return
		}
		// Must never panic; errors are fine.
		_, _ = cloudformation.ParseBytes(data, "fuzz.yaml", cloudformation.Options{
			Region: "us-east-1",
		})
	})
}
