// betmatic/notification_test.go

package betmatic

import (
	"encoding/json"
	"testing"
)

// A trimmed capture of a real GET /notification/ response — the resulted Fixed
// Profit bet whose numbers reconcile exactly: 730 accepted at an average 5.4795
// returns 3270, which is what makes total_accepted and profit dollars rather
// than counts. Everything the results poller reads is pinned here, because the
// only other way to find out a field moved is a wrong number in the bet channel.
const resultedNotification = `{
  "count": 1, "total_pages": 1, "next": null, "previous": null,
  "results": [{
    "id": 13026426,
    "tip": {
      "competition": {
        "code": "Galloping", "name": "ROCKHAMPTON", "event_number": 7,
        "runners": "1,2,4,5,6,8,10,11,13,14",
        "runner_names": "DIALIDAE#&#FUTURE SOLDIER#&#DOUBTWILLY#&#BLUE TOES#&#PEAKING TOM#&#BELVEDERE MISS#&#TARA I AM#&#TENNER#&#MAKING HISTORY#&#HARBOUR CHILL",
        "result": "10/4/6/14"
      },
      "market": "Fixed Win", "selection": "10", "odds": 1.7,
      "profit": 4.47945205479452,
      "result_time": "2026-08-13T17:17:54.399610+10:00"
    },
    "type": "Fixed Profit",
    "stake": 1, "target_profit": 560, "total_wager": 0,
    "total_accepted": 730, "average_odds": 5.47945205479452, "profit": 3270,
    "is_canceled": false,
    "created_at": "2026-08-13T17:10:50.633997+10:00",
    "triggered_at": "2026-08-13T17:10:50.640677+10:00",
    "label": "pegasus_Brank Daddy Pegasus"
  }]
}`

func TestResultedNotificationDecodes(t *testing.T) {
	var resp GetNotificationResponse
	if err := json.Unmarshal([]byte(resultedNotification), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(resp.Results))
	}

	r := resp.Results[0]

	if !r.Resulted() {
		t.Error("a notification with a result_time did not report as resulted")
	}
	if float64(r.TotalAccepted) != 730 {
		t.Errorf("total_accepted = %v, want 730", float64(r.TotalAccepted))
	}
	if float64(r.Profit) != 3270 {
		t.Errorf("profit = %v, want 3270", float64(r.Profit))
	}
	if float64(r.TargetProfit) != 560 {
		t.Errorf("target_profit = %v, want 560", float64(r.TargetProfit))
	}
	if r.Label != "pegasus_Brank Daddy Pegasus" {
		t.Errorf("label = %q", r.Label)
	}
	if r.Typ != FIXED_PROFIT {
		t.Errorf("type = %q, want %q", r.Typ, FIXED_PROFIT)
	}

	// accepted × (average_odds − 1) is the settled return, and tip.profit is the
	// same number per accepted dollar. If either drifts, the two stop agreeing.
	if want := float64(r.TotalAccepted) * (float64(r.AverageOdds) - 1); int(want) != int(float64(r.Profit)) {
		t.Errorf("profit %v does not reconcile with accepted × (odds−1) = %v", float64(r.Profit), want)
	}
	if int(float64(r.Tip.Profit)*100) != int((float64(r.AverageOdds)-1)*100) {
		t.Errorf("tip.profit %v is not average_odds−1", float64(r.Tip.Profit))
	}

	if got := r.Tip.Competition.Winner(); got != 10 {
		t.Errorf("winner = %d, want 10", got)
	}
	if got := r.Tip.Competition.RunnerName(10); got != "TARA I AM" {
		t.Errorf("runner 10 = %q, want TARA I AM", got)
	}
	if got := r.Tip.Competition.RunnerName(99); got != "" {
		t.Errorf("a runner not in the field resolved to %q", got)
	}
}

// The same shape for a beaten runner. Profit is signed — it is the settled
// loss, not the prospective return — which is the whole basis for reading
// WIN/LOSE off it, and it is negative accepted rather than negative units.
func TestLosingNotificationCarriesNegativeProfit(t *testing.T) {
	const losing = `{
      "id": 13025504,
      "tip": {
        "competition": {"name": "ROCKHAMPTON", "event_number": 5, "result": "8/5/7/1"},
        "market": "Fixed Win", "selection": "5", "odds": 1.7,
        "profit": -1,
        "result_time": "2026-08-13T15:57:56.546653+10:00"
      },
      "type": "Fixed Profit",
      "stake": 1, "target_profit": 420,
      "total_accepted": 220, "average_odds": 6.659090909090909, "profit": -220,
      "label": "pegasus_Brank Daddy Pegasus"
    }`

	var r Result
	if err := json.Unmarshal([]byte(losing), &r); err != nil {
		t.Fatal(err)
	}

	if !r.Resulted() {
		t.Error("a resulted loser did not report as resulted")
	}
	if float64(r.Profit) != -220 {
		t.Errorf("profit = %v, want -220", float64(r.Profit))
	}
	if float64(r.Profit) != -float64(r.TotalAccepted) {
		t.Errorf("a loser's profit %v is not the accepted stake %v", float64(r.Profit), float64(r.TotalAccepted))
	}
	if float64(r.Tip.Profit) != -1 {
		t.Errorf("tip.profit = %v, want -1 per accepted dollar", float64(r.Tip.Profit))
	}
	if got := r.Tip.Competition.Winner(); got == 5 {
		t.Error("the beaten selection was read as the winner")
	}
}

func TestUnresultedNotificationIsNotResulted(t *testing.T) {
	var r Result
	if err := json.Unmarshal([]byte(`{"id":1,"tip":{"result_time":null},"total_accepted":0}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.Resulted() {
		t.Error("a null result_time reported as resulted")
	}
}
