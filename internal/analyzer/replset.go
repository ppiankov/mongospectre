package analyzer

import (
	"fmt"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

// AuditReplicaSet checks replica set configuration for common misconfigurations.
// Returns nil if the info has no name (standalone or not a replica set).
func AuditReplicaSet(info mongoinspect.ReplicaSetInfo) []Finding {
	if info.Name == "" {
		return nil
	}
	var findings []Finding
	findings = append(findings, detectSingleMemberReplSet(&info)...)
	findings = append(findings, detectEvenMemberCount(&info)...)
	findings = append(findings, detectMemberUnhealthy(&info)...)
	findings = append(findings, detectOplogSmall(&info)...)
	findings = append(findings, detectNoHiddenMember(&info)...)
	findings = append(findings, detectPriorityZeroMajority(&info)...)
	return findings
}

func detectSingleMemberReplSet(info *mongoinspect.ReplicaSetInfo) []Finding {
	if len(info.Members) > 1 {
		return nil
	}
	return []Finding{{
		Type:     FindingSingleMemberReplSet,
		Severity: SeverityHigh,
		Message:  fmt.Sprintf("replica set %q has only %d member — no failover capability", info.Name, len(info.Members)),
	}}
}

func detectEvenMemberCount(info *mongoinspect.ReplicaSetInfo) []Finding {
	var votingMembers int
	for _, m := range info.Members {
		if m.Votes > 0 {
			votingMembers++
		}
	}
	if votingMembers == 0 || votingMembers%2 != 0 {
		return nil
	}
	return []Finding{{
		Type:     FindingEvenMemberCount,
		Severity: SeverityMedium,
		Message:  fmt.Sprintf("replica set %q has %d voting members — even count risks split-brain elections", info.Name, votingMembers),
	}}
}

var unhealthyStates = map[string]bool{
	"RECOVERING": true,
	"STARTUP":    true,
	"STARTUP2":   true,
	"DOWN":       true,
	"ROLLBACK":   true,
	"REMOVED":    true,
	"UNKNOWN":    true,
}

func detectMemberUnhealthy(info *mongoinspect.ReplicaSetInfo) []Finding {
	var findings []Finding
	for _, m := range info.Members {
		if m.Health == 0 || unhealthyStates[m.StateStr] {
			findings = append(findings, Finding{
				Type:     FindingMemberUnhealthy,
				Severity: SeverityHigh,
				Message:  fmt.Sprintf("replica set member %s is %s (health=%d)", m.Name, m.StateStr, m.Health),
			})
		}
	}
	return findings
}

func detectOplogSmall(info *mongoinspect.ReplicaSetInfo) []Finding {
	if info.OplogWindowHours <= 0 || info.OplogWindowHours >= 24 {
		return nil
	}
	return []Finding{{
		Type:     FindingOplogSmall,
		Severity: SeverityMedium,
		Message:  fmt.Sprintf("oplog window is %.1f hours — less than 24h increases risk of unrecoverable replication lag", info.OplogWindowHours),
	}}
}

func detectNoHiddenMember(info *mongoinspect.ReplicaSetInfo) []Finding {
	if len(info.Members) <= 3 {
		return nil
	}
	for _, m := range info.Members {
		if m.Hidden {
			return nil
		}
	}
	return []Finding{{
		Type:     FindingNoHiddenMember,
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("replica set %q has %d members but no hidden member for analytics or backup workloads", info.Name, len(info.Members)),
	}}
}

func detectPriorityZeroMajority(info *mongoinspect.ReplicaSetInfo) []Finding {
	if len(info.Members) == 0 {
		return nil
	}
	var zeroCount int
	for _, m := range info.Members {
		if m.Priority == 0 {
			zeroCount++
		}
	}
	if zeroCount*2 <= len(info.Members) {
		return nil
	}
	return []Finding{{
		Type:     FindingPriorityZeroMajority,
		Severity: SeverityHigh,
		Message:  fmt.Sprintf("replica set %q has %d/%d members with priority 0 — majority cannot become primary", info.Name, zeroCount, len(info.Members)),
	}}
}
