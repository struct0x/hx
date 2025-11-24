package out

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestProblem(t *testing.T) {
	pr := ProblemDetails{
		Type:       "type",
		StatusCode: 200,
		Title:      "title",
		Detail:     "details",
		Instance:   "instance",
		Extensions: map[string]any{
			"key":  "value",
			"key2": "value2",
		},
		Cause: fmt.Errorf("cause"),
	}

	expErr := "title: details; cause"

	if diff := cmp.Diff(expErr, pr.Error()); diff != "" {
		t.Errorf("problem details mismatch (-want +got):\n%s", diff)
	}
}

func TestProblemMarshaling(t *testing.T) {
	pr := ProblemDetails{
		Type:       "type",
		StatusCode: 200,
		Title:      "title",
		Detail:     "details",
		Instance:   "instance",
		Extensions: map[string]any{
			"key":  "value",
			"key2": "value2",
		},
	}

	bytes, err := pr.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	var pr2 ProblemDetails
	if err := pr2.UnmarshalJSON(bytes); err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(pr, pr2); diff != "" {
		t.Errorf("problem details mismatch (-want +got):\n%s", diff)
	}
}
