package main

import "testing"

func TestDecideFollowUp(t *testing.T) {
	tests := []struct {
		name     string
		job      FieldFollowUp
		schedule bool
	}{
		{"complete dispatch with photos", FieldFollowUp{OrderID: "WO-7", PhotoURLs: []string{"https://photos.example/7.jpg"}, DispatchStatus: "complete", FollowUpHours: 6}, true},
		{"dispatch still active", FieldFollowUp{OrderID: "WO-7", PhotoURLs: []string{"https://photos.example/7.jpg"}, DispatchStatus: "en_route", FollowUpHours: 6}, false},
		{"no inspection evidence", FieldFollowUp{OrderID: "WO-7", DispatchStatus: "complete", FollowUpHours: 6}, false},
		{"zero delay", FieldFollowUp{OrderID: "WO-7", PhotoURLs: []string{"photo"}, DispatchStatus: "complete"}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := decideFollowUp(test.job).Schedule
			if got != test.schedule {
				t.Fatalf("scheduled = %v, want %v", got, test.schedule)
			}
		})
	}
}
