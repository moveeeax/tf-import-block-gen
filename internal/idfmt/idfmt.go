// Package idfmt turns the id recorded in an inventory into the id Terraform
// actually expects in an import block. Every provider type has its own idea of
// what an id is: an instance imports by i-..., an aws_iam_role by the role
// name, an aws_sqs_queue by the queue URL, an aws_route53_record by a
// ZONEID_name_TYPE triple. That lookup table is the whole point of this file.
package idfmt

import (
	"fmt"
	"strings"
)

// Input is everything known about one candidate resource.
type Input struct {
	Type      string
	ID        string
	Name      string
	Region    string
	AccountID string
	Tags      map[string]string
	PlanAttrs map[string]any
}

// Result is the formatted import id plus whether the format is trusted.
type Result struct {
	ID       string
	Verified bool
	Note     string
}

type formatter func(Input) Result

var formatters = map[string]formatter{
	"aws_instance":             prefixed("i-"),
	"aws_vpc":                  prefixed("vpc-"),
	"aws_subnet":               prefixed("subnet-"),
	"aws_security_group":       prefixed("sg-"),
	"aws_internet_gateway":     prefixed("igw-"),
	"aws_nat_gateway":          prefixed("nat-"),
	"aws_s3_bucket":            bucketName,
	"aws_iam_role":             iamName("role/"),
	"aws_iam_user":             iamName("user/"),
	"aws_iam_instance_profile": iamName("instance-profile/"),
	"aws_iam_policy":           iamPolicyARN,
	"aws_dynamodb_table":       arnSegment("table/"),
	"aws_ecr_repository":       arnSegment("repository/"),
	"aws_cloudwatch_log_group": logGroupName,
	"aws_sqs_queue":            queueURL,
	"aws_route53_record":       route53Record,
}

// Supported lists the resource types with a known id format.
func Supported() []string {
	out := make([]string, 0, len(formatters))
	for t := range formatters {
		out = append(out, t)
	}
	return out
}

// Format returns the import id for in. An unknown type keeps the raw id and is
// marked unverified so the caller can flag it in the output.
func Format(in Input) Result {
	f, ok := formatters[in.Type]
	if !ok {
		return Result{
			ID:       in.ID,
			Verified: false,
			Note:     fmt.Sprintf("id format for %s is not verified -- check it before applying", in.Type),
		}
	}
	return f(in)
}

// prefixed covers the EC2-style types whose import id is the raw resource id.
func prefixed(prefix string) formatter {
	return func(in Input) Result {
		if strings.HasPrefix(in.ID, prefix) {
			return Result{ID: in.ID, Verified: true}
		}
		return Result{
			ID:       in.ID,
			Verified: false,
			Note:     fmt.Sprintf("%s imports by an id starting with %q; got %q", in.Type, prefix, in.ID),
		}
	}
}

// bucketName strips an S3 ARN down to the bucket name.
func bucketName(in Input) Result {
	if rest, ok := strings.CutPrefix(in.ID, "arn:aws:s3:::"); ok {
		return Result{ID: strings.TrimSuffix(rest, "/"), Verified: true}
	}
	if strings.HasPrefix(in.ID, "arn:") {
		return Result{ID: in.ID, Verified: false, Note: "aws_s3_bucket imports by bucket name; this ARN was not recognised"}
	}
	return Result{ID: in.ID, Verified: true}
}

// iamName pulls the trailing name out of an IAM ARN. Roles and users can live
// under a path (arn:aws:iam::1:role/team/app), and the import id is the last
// segment only.
func iamName(segment string) formatter {
	return func(in Input) Result {
		if !strings.HasPrefix(in.ID, "arn:") {
			return Result{ID: in.ID, Verified: true}
		}
		idx := strings.Index(in.ID, segment)
		if idx < 0 {
			return Result{ID: in.ID, Verified: false, Note: fmt.Sprintf("%s imports by name; %q has no %q segment", in.Type, in.ID, segment)}
		}
		rest := in.ID[idx+len(segment):]
		if i := strings.LastIndex(rest, "/"); i >= 0 {
			rest = rest[i+1:]
		}
		return Result{ID: rest, Verified: true}
	}
}

// iamPolicyARN is the awkward one: a managed policy imports by ARN, so a bare
// name can only be rebuilt when the inventory knows the account.
func iamPolicyARN(in Input) Result {
	if strings.HasPrefix(in.ID, "arn:") {
		return Result{ID: in.ID, Verified: true}
	}
	if in.AccountID != "" {
		return Result{ID: fmt.Sprintf("arn:aws:iam::%s:policy/%s", in.AccountID, in.ID), Verified: true}
	}
	return Result{
		ID:       in.ID,
		Verified: false,
		Note:     "aws_iam_policy imports by ARN; the inventory gave a bare name and no account_id",
	}
}

// arnSegment keeps the part after segment, for ARNs shaped
// arn:aws:svc:region:acct:<segment><name>.
func arnSegment(segment string) formatter {
	return func(in Input) Result {
		if idx := strings.Index(in.ID, segment); idx >= 0 {
			return Result{ID: in.ID[idx+len(segment):], Verified: true}
		}
		if strings.HasPrefix(in.ID, "arn:") {
			return Result{ID: in.ID, Verified: false, Note: fmt.Sprintf("%s: %q has no %q segment", in.Type, in.ID, segment)}
		}
		return Result{ID: in.ID, Verified: true}
	}
}

// logGroupName handles the ":log-group:/name:*" tail CloudWatch ARNs carry.
func logGroupName(in Input) Result {
	const seg = ":log-group:"
	if idx := strings.Index(in.ID, seg); idx >= 0 {
		return Result{ID: strings.TrimSuffix(in.ID[idx+len(seg):], ":*"), Verified: true}
	}
	if strings.HasPrefix(in.ID, "arn:") {
		return Result{ID: in.ID, Verified: false, Note: "aws_cloudwatch_log_group: ARN has no :log-group: segment"}
	}
	return Result{ID: in.ID, Verified: true}
}

// queueURL builds the queue URL an aws_sqs_queue import needs.
func queueURL(in Input) Result {
	if strings.HasPrefix(in.ID, "https://") {
		return Result{ID: in.ID, Verified: true}
	}
	if parts := strings.Split(in.ID, ":"); len(parts) == 6 && parts[0] == "arn" && parts[2] == "sqs" {
		return Result{ID: fmt.Sprintf("https://sqs.%s.amazonaws.com/%s/%s", parts[3], parts[4], parts[5]), Verified: true}
	}
	if in.Region != "" && in.AccountID != "" {
		return Result{ID: fmt.Sprintf("https://sqs.%s.amazonaws.com/%s/%s", in.Region, in.AccountID, in.ID), Verified: true}
	}
	return Result{
		ID:       in.ID,
		Verified: false,
		Note:     "aws_sqs_queue imports by queue URL; the inventory gave neither a URL nor region + account_id",
	}
}

// route53Record composes the ZONEID_name_TYPE id, preferring an id that is
// already composite and otherwise reading zone_id/name/type off the plan.
func route53Record(in Input) Result {
	if strings.Count(in.ID, "_") >= 2 {
		return Result{ID: in.ID, Verified: true}
	}
	zone := attr(in.PlanAttrs, "zone_id")
	name := attr(in.PlanAttrs, "name")
	rtype := attr(in.PlanAttrs, "type")
	if zone != "" && name != "" && rtype != "" {
		return Result{
			ID:       fmt.Sprintf("%s_%s_%s", zone, strings.TrimSuffix(name, "."), strings.ToUpper(rtype)),
			Verified: true,
		}
	}
	return Result{
		ID:       in.ID,
		Verified: false,
		Note:     "aws_route53_record imports by ZONEID_name_TYPE; could not compose one from the plan",
	}
}

func attr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}
