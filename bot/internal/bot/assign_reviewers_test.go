package bot

import "testing"

func TestLimitAdditionalReviewers(t *testing.T) {
	wanted := []string{"alice", "bob"}

	if got := limitAdditionalReviewers(wanted, nil, "author", 2); len(got) != 2 {
		t.Fatalf("nobody requested yet should keep both, got %v", got)
	}

	got := limitAdditionalReviewers(wanted, []string{"carol"}, "author", 2)
	if len(got) != 1 || got[0] != "alice" {
		t.Fatalf("one reviewer already requested should add only one more, got %v", got)
	}

	if got := limitAdditionalReviewers(wanted, []string{"carol", "dave"}, "author", 2); got != nil {
		t.Fatalf("two reviewers already requested should add none, got %v", got)
	}

	got = limitAdditionalReviewers(wanted, []string{"alice"}, "author", 2)
	if len(got) != 1 || got[0] != "bob" {
		t.Fatalf("already-requested reviewer should not be requested again, got %v", got)
	}
}
