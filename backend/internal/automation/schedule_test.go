package automation

import (
	"testing"
	"time"
)

func TestSchedule(t *testing.T) {
	w := DefaultWindow()
	loc, _ := time.LoadLocation(w.Timezone)
	fri := time.Date(2026, 3, 27, 16, 0, 0, 0, loc)
	due, e := BusinessDays(fri, 2, w)
	if e != nil || due.Weekday() != time.Tuesday || due.Hour() != 16 || due.Day() != 31 {
		t.Fatalf("DST business days: %v %v", due, e)
	}
	accepted := due.AddDate(0, 0, 1)
	first, _ := FirstMessage(fri, accepted, w)
	if !first.Equal(accepted) {
		t.Fatal(first)
	}
	if _, e = FirstMessage(fri, time.Time{}, w); e == nil {
		t.Fatal("unaccepted invitation allowed")
	}
	for days, want := range map[int]int{0: 40, 13: 40, 14: 60, 59: 60, 60: 100} {
		if DailyLimit(fri, fri.Add(time.Duration(days)*24*time.Hour)) != want {
			t.Fatal(days)
		}
	}
	if MaySend(Facts{Replied: true}) {
		t.Fatal("reply must stop workflow")
	}
	if ok, e := Evaluate(Condition{Kind: "ai"}, Facts{}); ok || e == nil {
		t.Fatal("missing AI decision")
	}
}
func TestCustomDays(t *testing.T) {
	w := DefaultWindow()
	w.WorkingDays = []int{int(time.Saturday)}
	now := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	next, e := NextActive(now, w)
	if e != nil || next.Weekday() != time.Saturday {
		t.Fatal(next, e)
	}
	next, e = BusinessDays(next, 1, w)
	if e != nil || next.Day() != 17 {
		t.Fatal(next, e)
	}
	w.WorkingDays = []int{}
	if _, e = NextActive(now, w); e == nil {
		t.Fatal("empty workweek allowed")
	}
}
