package termux

import (
	"strings"
	"testing"
)

func TestParseGetprop(t *testing.T) {
	dump := `[ro.build.version.opporom]: [V13.0]
[ro.product.manufacturer]: [OPPO]
[ro.build.display.id]: [CPH2449_14.0.0.660(EXC01)]
[garbage line]
[empty.prop]: []
`
	props := ParseGetprop(dump)
	if props["ro.build.version.opporom"] != "V13.0" {
		t.Errorf("opporom = %q", props["ro.build.version.opporom"])
	}
	if props["ro.product.manufacturer"] != "OPPO" {
		t.Errorf("manufacturer = %q", props["ro.product.manufacturer"])
	}
	if v, ok := props["empty.prop"]; !ok || v != "" {
		t.Errorf("empty.prop = %q ok=%v", v, ok)
	}
	if _, ok := props["garbage line"]; ok {
		t.Error("garbage line should be skipped")
	}
	// Only the outermost bracket pair is stripped: inner brackets must be
	// preserved (e.g. custom ROM display IDs like "[HyperOS]_V1").
	dump2 := `[ro.build.display.id]: [[HyperOS]_V1]
[ro.bracket.edge]: []]
`
	props2 := ParseGetprop(dump2)
	if v := props2["ro.build.display.id"]; v != "[HyperOS]_V1" {
		t.Errorf("display.id = %q, want %q", v, "[HyperOS]_V1")
	}
	if v := props2["ro.bracket.edge"]; v != "]" {
		t.Errorf("bracket.edge = %q, want %q", v, "]")
	}
}

func TestClassifyOEMFamilies(t *testing.T) {
	cases := []struct {
		name    string
		props   map[string]string
		want    string
		aggress bool
	}{
		{"coloros-oppo", map[string]string{"ro.build.version.opporom": "V13.0"}, OEMColorOS, true},
		{"coloros-oneplus-manuf", map[string]string{"ro.product.manufacturer": "OnePlus"}, OEMColorOS, true},
		{"miui-prop", map[string]string{"ro.miui.ui.version.name": "V140"}, OEMMIUI, true},
		{"miui-displayid", map[string]string{"ro.build.display.id": "HyperOS_V816"}, OEMMIUI, true},
		{"miui-redmi-manuf-lower", map[string]string{"ro.product.manufacturer": "Xiaomi"}, OEMMIUI, true},
		{"originos", map[string]string{"ro.vivo.os.name": "OriginOS"}, OEMOriginOS, true},
		{"emui", map[string]string{"ro.build.version.emui": "Emui_12.0"}, OEMEMUI, true},
		// EMUI API levels are often plain integers ("9", "11") with no dot:
		// presence alone must classify the device (review issue #5).
		{"emui-api-level-integer", map[string]string{"ro.build.hw_emui_api_level": "11"}, OEMEMUI, true},
		{"flyme", map[string]string{"ro.product.manufacturer": "Meizu"}, OEMFlyme, true},
		{"other-asus", map[string]string{"ro.product.manufacturer": "Asus"}, OEMOther, true},
		{"pixel-clean", map[string]string{"ro.product.manufacturer": "Google", "ro.build.display.id": "TQ3A.230901"}, OEMUnknown, false},
		{"samsung-not-flagged", map[string]string{"ro.product.manufacturer": "samsung"}, OEMUnknown, false},
		{"empty", map[string]string{}, OEMUnknown, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := classifyOEM(tc.props)
			if info.Family != tc.want {
				t.Errorf("Family = %q, want %q", info.Family, tc.want)
			}
			if info.Aggressive != tc.aggress {
				t.Errorf("Aggressive = %v, want %v", info.Aggressive, tc.aggress)
			}
		})
	}
}

func TestChecklistForAggressiveFamilies(t *testing.T) {
	for _, fam := range []string{OEMColorOS, OEMMIUI, OEMOriginOS, OEMEMUI, OEMFlyme, OEMOther} {
		info := &OEMInfo{Family: fam, Aggressive: true}
		list := info.Checklist()
		if len(list) < 2 {
			t.Errorf("family %q checklist too short: %d", fam, len(list))
		}
		joined := strings.Join(list, "\n")
		if !strings.Contains(joined, "Termux") {
			t.Errorf("family %q checklist must mention Termux whitelisting", fam)
		}
	}
	// Unknown / non-aggressive yields no checklist.
	if c := (&OEMInfo{}).Checklist(); c != nil {
		t.Errorf("non-aggressive checklist = %v, want nil", c)
	}
}

func TestManufacturerMatchingIsCaseInsensitive(t *testing.T) {
	// getprop values may come with arbitrary casing.
	info := classifyOEM(map[string]string{"ro.product.manufacturer": "OPPO"})
	if info.Family != OEMColorOS {
		t.Errorf("uppercase OPPO not matched: %q", info.Family)
	}
}
