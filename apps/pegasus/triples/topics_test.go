// triple-s/topics_test.go

package triples

import "testing"

func TestTopicCode(t *testing.T) {
	cases := []struct {
		topic, want string
	}{
		{"PROD/PRDF/AUS/RVIC/FLEMINGTON/3", "AUS/RVIC"},
		{"PROD/PRDF/AUS/HRNSW/MENANGLE/7", "AUS/HRNSW"},
		{"PROD/PRDF/AUS/RVIC", "AUS/RVIC"},
		{"PROD/PRDF/AUS/RVIC/", "AUS/RVIC"},
		{"PROD/PRDF/AUS", ""},
		{"PROD/PRDF", ""},
		{"", ""},
	}

	for _, c := range cases {
		if got := TopicCode(c.topic); got != c.want {
			t.Errorf("TopicCode(%q) = %q, want %q", c.topic, got, c.want)
		}
	}
}
