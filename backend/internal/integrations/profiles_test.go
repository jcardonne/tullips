package integrations

import (
	"strings"
	"testing"
)

func TestProfilesResolveSharedText(t *testing.T) {
	raw := []byte(`0:{"triggers":[{"action":{"value":{"url":"https://www.linkedin.com/in/aster-bloom/"}}}],"children":"$L2"}
2:["$","div",null,{"children":["$L3","$L4","$L5"]}]
3:["$","$L54",null,{"textProps":{"children":["Aster Bloom",["$","span",null,{"children":"2nd"}]]}}]
4:["$","$L54",null,{"textProps":{"children":["Operations director"]}}]
5:["$","$L54",null,{"textProps":{"children":["Paris, France"]}}]`)
	profiles := ParseProspects(raw)
	if len(profiles) != 1 || profiles[0].Name != "Aster Bloom" || profiles[0].Headline != "Operations director" || profiles[0].Location != "Paris, France" {
		t.Fatalf("%+v", profiles)
	}
	if profiles[0].Company != "" {
		t.Fatal("invented company")
	}
	// A partial card must not borrow a different prospect's rich text from a parent.
	raw = []byte(`0:{"children":[{"triggers":[{"url":"https://www.linkedin.com/in/empty/"}],"children":"$L6"},{"triggers":[{"url":"https://www.linkedin.com/in/aster-bloom/"}],"children":"$L2"}]}
2:["$","div",null,{"children":["$L3","$L4","$L5"]}]
3:["$","$L54",null,{"textProps":{"children":["Aster Bloom"]}}]
4:["$","$L54",null,{"textProps":{"children":["Operations director"]}}]
5:["$","$L54",null,{"textProps":{"children":["Paris, France"]}}]
6:["$","div",null,{"children":[]}]`)
	for _, p := range ParseProspects(raw) {
		if strings.Contains(p.URL, "empty") && p.Name != "" {
			t.Fatal("cross-profile metadata leak")
		}
	}
}
