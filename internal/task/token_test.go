package task

import (
	"testing"
)

func TestGenerateLeaseToken(t *testing.T) {
	s, err := generateLeaseToken()
	if err != nil {
		t.Fatalf("generateLeaseToken failed:%v", err)
	}
	if len(s) != 32 {
		t.Fatalf("token length != 32")
	}
	q, err := generateLeaseToken()
	if err != nil {
		t.Fatalf("generateLeaseToken failed:%v", err)
	}
	if len(q) != 32 {
		t.Fatalf("token length != 32")
	}
	if q == s {
		t.Fatalf("two token are same")
	}
}
