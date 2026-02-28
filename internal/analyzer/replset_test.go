package analyzer

import (
	"testing"

	mongoinspect "github.com/ppiankov/mongospectre/internal/mongo"
)

func TestAuditReplicaSet_Empty(t *testing.T) {
	findings := AuditReplicaSet(mongoinspect.ReplicaSetInfo{})
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for empty info, got %d", len(findings))
	}
}

func TestDetectSingleMemberReplSet(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", StateStr: "PRIMARY", Health: 1, Votes: 1, Priority: 1},
		},
	}
	findings := detectSingleMemberReplSet(&info)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingSingleMemberReplSet {
		t.Fatalf("expected SINGLE_MEMBER_REPLSET, got %s", findings[0].Type)
	}
}

func TestDetectSingleMemberReplSet_Multi(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", StateStr: "PRIMARY", Health: 1, Votes: 1, Priority: 1},
			{Name: "host2:27017", StateStr: "SECONDARY", Health: 1, Votes: 1, Priority: 1},
			{Name: "host3:27017", StateStr: "SECONDARY", Health: 1, Votes: 1, Priority: 1},
		},
	}
	findings := detectSingleMemberReplSet(&info)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestDetectEvenMemberCount(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", Votes: 1},
			{Name: "host2:27017", Votes: 1},
		},
	}
	findings := detectEvenMemberCount(&info)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingEvenMemberCount {
		t.Fatalf("expected EVEN_MEMBER_COUNT, got %s", findings[0].Type)
	}
}

func TestDetectEvenMemberCount_Odd(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", Votes: 1},
			{Name: "host2:27017", Votes: 1},
			{Name: "host3:27017", Votes: 1},
		},
	}
	findings := detectEvenMemberCount(&info)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for odd voting count, got %d", len(findings))
	}
}

func TestDetectMemberUnhealthy(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", StateStr: "PRIMARY", Health: 1},
			{Name: "host2:27017", StateStr: "RECOVERING", Health: 1},
		},
	}
	findings := detectMemberUnhealthy(&info)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingMemberUnhealthy {
		t.Fatalf("expected MEMBER_UNHEALTHY, got %s", findings[0].Type)
	}
}

func TestDetectMemberUnhealthy_AllHealthy(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", StateStr: "PRIMARY", Health: 1},
			{Name: "host2:27017", StateStr: "SECONDARY", Health: 1},
		},
	}
	findings := detectMemberUnhealthy(&info)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestDetectMemberUnhealthy_HealthZero(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", StateStr: "PRIMARY", Health: 1},
			{Name: "host2:27017", StateStr: "SECONDARY", Health: 0},
		},
	}
	findings := detectMemberUnhealthy(&info)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestDetectOplogSmall(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name:             "rs0",
		OplogWindowHours: 12.5,
	}
	findings := detectOplogSmall(&info)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingOplogSmall {
		t.Fatalf("expected OPLOG_SMALL, got %s", findings[0].Type)
	}
}

func TestDetectOplogSmall_Adequate(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name:             "rs0",
		OplogWindowHours: 48.0,
	}
	findings := detectOplogSmall(&info)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestDetectNoHiddenMember(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", Hidden: false},
			{Name: "host2:27017", Hidden: false},
			{Name: "host3:27017", Hidden: false},
			{Name: "host4:27017", Hidden: false},
		},
	}
	findings := detectNoHiddenMember(&info)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingNoHiddenMember {
		t.Fatalf("expected NO_HIDDEN_MEMBER, got %s", findings[0].Type)
	}
}

func TestDetectNoHiddenMember_HasHidden(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", Hidden: false},
			{Name: "host2:27017", Hidden: false},
			{Name: "host3:27017", Hidden: false},
			{Name: "host4:27017", Hidden: true},
		},
	}
	findings := detectNoHiddenMember(&info)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestDetectNoHiddenMember_SmallSet(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", Hidden: false},
			{Name: "host2:27017", Hidden: false},
			{Name: "host3:27017", Hidden: false},
		},
	}
	findings := detectNoHiddenMember(&info)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for <=3 members, got %d", len(findings))
	}
}

func TestDetectPriorityZeroMajority(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", Priority: 1},
			{Name: "host2:27017", Priority: 0},
			{Name: "host3:27017", Priority: 0},
		},
	}
	findings := detectPriorityZeroMajority(&info)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Type != FindingPriorityZeroMajority {
		t.Fatalf("expected PRIORITY_ZERO_MAJORITY, got %s", findings[0].Type)
	}
}

func TestDetectPriorityZeroMajority_Ok(t *testing.T) {
	info := mongoinspect.ReplicaSetInfo{
		Name: "rs0",
		Members: []mongoinspect.ReplicaSetMember{
			{Name: "host1:27017", Priority: 1},
			{Name: "host2:27017", Priority: 1},
			{Name: "host3:27017", Priority: 0},
		},
	}
	findings := detectPriorityZeroMajority(&info)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}
