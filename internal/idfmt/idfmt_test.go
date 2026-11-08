package idfmt

import "testing"

func TestFormat(t *testing.T) {
	tests := []struct {
		name     string
		in       Input
		wantID   string
		verified bool
	}{
		{
			name:     "s3 arn becomes bucket name",
			in:       Input{Type: "aws_s3_bucket", ID: "arn:aws:s3:::acme-assets"},
			wantID:   "acme-assets",
			verified: true,
		},
		{
			name:     "s3 bare name passes through",
			in:       Input{Type: "aws_s3_bucket", ID: "acme-assets"},
			wantID:   "acme-assets",
			verified: true,
		},
		{
			name:     "iam role path is stripped to the last segment",
			in:       Input{Type: "aws_iam_role", ID: "arn:aws:iam::1:role/team/app-task"},
			wantID:   "app-task",
			verified: true,
		},
		{
			name:     "iam policy arn is kept",
			in:       Input{Type: "aws_iam_policy", ID: "arn:aws:iam::1:policy/ro"},
			wantID:   "arn:aws:iam::1:policy/ro",
			verified: true,
		},
		{
			name:     "iam policy name plus account becomes an arn",
			in:       Input{Type: "aws_iam_policy", ID: "ro", AccountID: "123456789012"},
			wantID:   "arn:aws:iam::123456789012:policy/ro",
			verified: true,
		},
		{
			name:     "iam policy name without an account is unverified",
			in:       Input{Type: "aws_iam_policy", ID: "ro"},
			wantID:   "ro",
			verified: false,
		},
		{
			name:     "instance id is taken as is",
			in:       Input{Type: "aws_instance", ID: "i-0abc"},
			wantID:   "i-0abc",
			verified: true,
		},
		{
			name:     "instance arn is flagged",
			in:       Input{Type: "aws_instance", ID: "arn:aws:ec2:eu-central-1:1:instance/i-0abc"},
			wantID:   "arn:aws:ec2:eu-central-1:1:instance/i-0abc",
			verified: false,
		},
		{
			name:     "sqs arn becomes a queue url",
			in:       Input{Type: "aws_sqs_queue", ID: "arn:aws:sqs:eu-central-1:123456789012:jobs"},
			wantID:   "https://sqs.eu-central-1.amazonaws.com/123456789012/jobs",
			verified: true,
		},
		{
			name:     "sqs name plus region and account becomes a queue url",
			in:       Input{Type: "aws_sqs_queue", ID: "jobs", Region: "eu-west-1", AccountID: "1"},
			wantID:   "https://sqs.eu-west-1.amazonaws.com/1/jobs",
			verified: true,
		},
		{
			name:     "sqs bare name alone is unverified",
			in:       Input{Type: "aws_sqs_queue", ID: "jobs"},
			wantID:   "jobs",
			verified: false,
		},
		{
			name:     "log group arn loses the :* suffix",
			in:       Input{Type: "aws_cloudwatch_log_group", ID: "arn:aws:logs:eu-central-1:1:log-group:/acme/app:*"},
			wantID:   "/acme/app",
			verified: true,
		},
		{
			name:     "dynamodb arn becomes the table name",
			in:       Input{Type: "aws_dynamodb_table", ID: "arn:aws:dynamodb:eu-central-1:1:table/sessions"},
			wantID:   "sessions",
			verified: true,
		},
		{
			name:     "ecr arn becomes the repository name",
			in:       Input{Type: "aws_ecr_repository", ID: "arn:aws:ecr:eu-central-1:1:repository/api"},
			wantID:   "api",
			verified: true,
		},
		{
			name: "route53 record is composed from the plan",
			in: Input{
				Type: "aws_route53_record", ID: "acme.example.com",
				PlanAttrs: map[string]any{"zone_id": "Z1D633PJN98FT9", "name": "acme.example.com.", "type": "a"},
			},
			wantID:   "Z1D633PJN98FT9_acme.example.com_A",
			verified: true,
		},
		{
			name:     "an already composite route53 id is kept",
			in:       Input{Type: "aws_route53_record", ID: "Z1_acme.example.com_CNAME"},
			wantID:   "Z1_acme.example.com_CNAME",
			verified: true,
		},
		{
			name:     "route53 without plan attributes is unverified",
			in:       Input{Type: "aws_route53_record", ID: "acme.example.com"},
			wantID:   "acme.example.com",
			verified: false,
		},
		{
			name:     "an unknown type keeps the raw id and is unverified",
			in:       Input{Type: "cloudflare_record", ID: "abc123"},
			wantID:   "abc123",
			verified: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Format(tc.in)
			if got.ID != tc.wantID {
				t.Errorf("id = %q, want %q", got.ID, tc.wantID)
			}
			if got.Verified != tc.verified {
				t.Errorf("verified = %v, want %v (note: %s)", got.Verified, tc.verified, got.Note)
			}
			if !got.Verified && got.Note == "" {
				t.Error("an unverified result must carry a note")
			}
		})
	}
}

func TestSupportedIsNotEmpty(t *testing.T) {
	if len(Supported()) < 10 {
		t.Fatalf("only %d types have a known id format", len(Supported()))
	}
}
