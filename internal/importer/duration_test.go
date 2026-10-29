package importer

import "testing"

func ip(n int) *int { return &n }

func eqInt(a, b *int) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func show(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func showS(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestParseISODuration(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
		ok   bool
	}{
		{"PT1H30M", 90, true},
		{"PT45M", 45, true},
		{"pt20m", 20, true},
		{"PT2H", 120, true},
		{"P1DT2H", 1560, true},
		{"P1W", 10080, true},
		{"PT0.5H", 30, true},
		{"PT90S", 2, true},
		{"PT30S", 0, true}, // 0.5 rounds half to even
		{"P0Y0M0DT0H35M0.000S", 35, true},
		{"P0DT0H0M", 0, true},
		{"PT", 0, true},
		{"P", 0, true},
		{" PT10M ", 10, true},
		{"P1M", 0, false}, // a month is not a cooking time
		{"P1Y", 0, false},
		{"PT1H30", 0, false},
		{"1 hour", 0, false},
		{"", 0, false},
	} {
		got, ok := ParseISODuration(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ParseISODuration(%q) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestMinutes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want *int
	}{
		{"45", ip(45)},
		{"1:10", ip(70)},
		{"1 hour 15 minutes", ip(75)},
		{"1h", ip(60)},
		{"1 hr 5 mins", ip(65)},
		{"1 hour and 15 minutes", ip(75)},
		{"1 hour, 15 minutes", ip(75)},
		{"about 20 min", ip(20)},
		{"Prep time: 30 min.", ip(30)},
		{"prep/total time: 30 min.", ip(30)},
		{"1½ hours", ip(90)},
		{"1 1/2 hours", ip(90)},
		{"1/2 hour", ip(30)},
		{"2.5 hrs", ip(150)},
		{"0", nil},
		{"0 minutes", nil},
		{"", nil},
		{"20-30 minutes", nil},
		{"2 hours plus overnight", nil},
		{"at least 4 hours", nil},
		{"45 minutes (plus chilling)", nil},
		{"1 hand 5 min", nil}, // "h" must not be followed by a letter
		{"5 min.x", nil},
		{"12 hoursy", nil},
		{"PT30M", nil}, // Mela wording only; ISO is ParseISODuration
	} {
		if got := Minutes(tc.in); !eqInt(got, tc.want) {
			t.Errorf("Minutes(%q) = %v; want %v", tc.in, show(got), show(tc.want))
		}
	}
}

func TestTimes(t *testing.T) {
	for _, tc := range []struct {
		name              string
		prep, cook, total string
		p, c, tt          *int
		text              *string
	}{
		{"iso", "PT15M", "PT1H", "PT1H15M", ip(15), ip(60), ip(75), nil},
		{"no total invented", "PT15M", "PT30M", "", ip(15), ip(30), nil, nil},
		{"zero total", "PT10M", "", "PT0S", ip(10), nil, nil, nil},
		{"mela wording", "15 mins", "1 hour", "", ip(15), ip(60), nil, nil},
		{"lone loose total bare", "", "", "2 hours plus overnight", nil, nil, nil, sp("2 hours plus overnight")},
		{"loose labelled", "10 to 15 minutes", "PT20M", "about 1 hour, plus chilling", nil, ip(20), nil, sp("Prep: 10 to 15 minutes · Total: about 1 hour, plus chilling")},
		{"loose cook only", "", "overnight", "", nil, nil, nil, sp("Cook: overnight")},
		{"none", "", "", "", nil, nil, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, c, tt, text := Times(tc.prep, tc.cook, tc.total)
			if !eqInt(p, tc.p) || !eqInt(c, tc.c) || !eqInt(tt, tc.tt) {
				t.Errorf("minutes = %v %v %v; want %v %v %v", show(p), show(c), show(tt), show(tc.p), show(tc.c), show(tc.tt))
			}
			if showS(text) != showS(tc.text) {
				t.Errorf("text = %v; want %v", showS(text), showS(tc.text))
			}
		})
	}
}

func sp(s string) *string { return &s }
