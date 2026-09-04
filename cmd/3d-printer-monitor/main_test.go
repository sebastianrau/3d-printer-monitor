package main

import "testing"

func TestNotificationSummaryIsStableAndContainsNoImplicitValues(t *testing.T) {
	if got := notificationSummary(nil); got != "defaults" {
		t.Fatalf("nil notification summary = %q", got)
	}
	got := notificationSummary(map[string]bool{"started": true, "finished": false, "layer1": true})
	if got != "finished=false,layer1=true,started=true" {
		t.Fatalf("notification summary = %q", got)
	}
}
