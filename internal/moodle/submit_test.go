package moodle

import (
	"context"
	"net/http"
	"testing"
)

// An assignment without drafts takes the work as submitted on save, and Moodle then
// refuses mod_assign_submit_for_grading; asking to submit must still succeed.
func TestSaveSubmissionAlreadySubmittedOnSave(t *testing.T) {
	old := http.DefaultClient.Transport
	http.DefaultClient.Transport = fakeWS{
		"mod_assign_save_submission":       `[]`,
		"mod_assign_get_submission_status": `{"lastattempt":{"submission":{"status":"submitted"}}}`,
		"mod_assign_submit_for_grading":    `[{"item":"assign","warningcode":"couldnotsubmitforgrading","message":"Could not submit assignment for grading."}]`,
	}
	t.Cleanup(func() { http.DefaultClient.Transport = old })

	c := &Client{token: "t", userID: 1}
	if err := c.SaveSubmission(context.Background(), 18657, nil, "ответ", true); err != nil {
		t.Fatalf("SaveSubmission = %v, want nil: the work is already submitted", err)
	}
}

func TestSaveSubmissionSubmitsDraft(t *testing.T) {
	old := http.DefaultClient.Transport
	http.DefaultClient.Transport = fakeWS{
		"mod_assign_save_submission":       `[]`,
		"mod_assign_get_submission_status": `{"lastattempt":{"submission":{"status":"draft"}}}`,
		"mod_assign_submit_for_grading":    `[{"item":"assign","warningcode":"couldnotsubmitforgrading","message":"Could not submit assignment for grading."}]`,
	}
	t.Cleanup(func() { http.DefaultClient.Transport = old })

	c := &Client{token: "t", userID: 1}
	if err := c.SaveSubmission(context.Background(), 1, nil, "ответ", true); err == nil {
		t.Fatal("SaveSubmission = nil, want the submit_for_grading warning for a draft")
	}
}
