package filter

import "testing"

func TestValidateCheckmateExclusiveWithOtherEndStates(t *testing.T) {
	if err := (&Options{Checkmate: true}).Validate(); err != nil {
		t.Fatalf("checkmate alone: %v", err)
	}
	if err := (&Options{Stalemate: true, Insufficient: true}).Validate(); err != nil {
		t.Fatalf("stalemate with insufficient: %v", err)
	}
	for _, o := range []*Options{
		{Checkmate: true, Stalemate: true},
		{Checkmate: true, Insufficient: true},
	} {
		if err := o.Validate(); err == nil {
			t.Errorf("%+v: want error", o)
		}
	}
}
