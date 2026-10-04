package core

import "testing"

func TestQualificationIsEvidenceBound(t *testing.T) {
	allowed := map[string]bool{"https://linkedin.com/in/ada": true}
	rows, e := parseQualifications(`[{"url":"https://linkedin.com/in/ada","match":null,"score":40,"reason":"Location not observed"}]`, allowed)
	if e != nil || rows["https://linkedin.com/in/ada"].Match != nil {
		t.Fatal("uncertain profile must not qualify")
	}
	if _, e = parseQualifications(`[{"url":"https://linkedin.com/in/ada","match":"true","score":90,"reason":"claimed"}]`, allowed); e == nil {
		t.Fatal("string true must not qualify")
	}
	if _, e = parseQualifications(`[{"url":"https://linkedin.com/in/unknown","match":true,"score":99,"reason":"invented"}]`, allowed); e == nil {
		t.Fatal("invented profile accepted")
	}
}
