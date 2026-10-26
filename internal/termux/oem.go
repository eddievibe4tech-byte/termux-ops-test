// OEM ROM detection: aggressive battery managers (ColorOS, MIUI/HyperOS,
// OriginOS, EMUI, Flyme, …) ship "task killer" mechanisms that freeze or
// kill Termux despite wake locks, causing scheduled backups to silently
// not run (issue #43). This file probes well-known Android system properties
// via getprop (or by parsing a captured build.prop on non-Android hosts,
// which keeps the logic unit-testable) and classifies the device so that
// `hlops doctor` can show an OEM-specific survival checklist.
package termux

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// OEM family identifiers returned by DetectOEM().
const (
	OEMUnknown  = ""               // stock Android / unsupported probe environment
	OEMColorOS  = "ColorOS"        // OPPO / OnePlus / realme
	OEMMIUI     = "HyperOS/MIUI"   // Xiaomi / Redmi / POCO
	OEMOriginOS = "OriginOS"       // vivo / iQOO
	OEMEMUI     = "EMUI/HarmonyOS" // Huawei / Honor
	OEMFlyme    = "Flyme"          // Meizu
	OEMOther    = "other"          // recognised as custom ROM, family unknown
)

// The properties we rely on: since Android 11 OPPO / OnePlus / realme share
// the Oplus codebase, so ro.build.version.oplusrom is the primary ColorOS /
// OxygenOS / realme UI marker (ro.build.version.opporom and
// ro.build.version.realmeui remain as legacy props); MIUI uses
// ro.miui.ui.version.name while HyperOS introduced ro.mi.os.version.name;
// vivo variants expose ro.vivo.os.name / ro.vivo.os.version; Huawei uses
// ro.build.version.emui / ro.build.hw_emui_api_level and Honor after its
// spin-off uses ro.build.version.magic. classifyOEM below reads them from a
// full `getprop` dump.

// OEMInfo is the classification result for one device.
type OEMInfo struct {
	// Family is one of the OEM* constants above (OEMUnknown when no
	// aggressive task killer was detected).
	Family string
	// Manufacturer / Brand echo the raw getprop values (original casing
	// preserved for display); classification compares case-insensitively.
	Manufacturer string
	Brand        string
	// DisplayID is ro.build.display.id when available.
	DisplayID string
	// Aggressive reports whether this family is known for killing
	// background processes despite PARTIAL_WAKE_LOCK (issue #43).
	Aggressive bool
}

// Checklist returns the per-OEM action items shown by `hlops doctor`. The
// entries are deliberately short imperative sentences; the long-form SOP
// lives in docs/termux-quickstart.md ("防殺背景設定 SOP").
func (o *OEMInfo) Checklist() []string {
	if o == nil || !o.Aggressive {
		return nil
	}
	generic := []string{
		"電池最佳化：設定 → 應用程式 → Termux → 耗電量管理 → 允許完全背景行為（不最佳化）",
		"多工鎖定：在最近任務畫面將 Termux 卡片上鎖，防止一鍵清理終止",
		"排程於深夜時，關閉「睡眠待機最佳化 / 夜間省電」",
	}
	switch o.Family {
	case OEMColorOS:
		return append([]string{
			"ColorOS：設定 → 電池 → 更多設定 → 關閉「智慧節能」與「夜間深度休眠」",
			"ColorOS：設定 → 應用程式 → 特殊應用程式存取權 → 啟動管理 → 允許 Termux 自啟動與關聯啟動",
		}, generic...)
	case OEMMIUI:
		return append([]string{
			"MIUI/HyperOS：設定 → 應用設定 → 特殊權限 → 自啟動管理 → 允許 Termux",
			"MIUI/HyperOS：長按 Termux 圖示 → 應用資訊 → 省電策略 → 無限制",
			"MIUI/HyperOS：安全中心 → 掃描加速 → 將 Termux 加入白名單",
		}, generic...)
	case OEMOriginOS:
		return append([]string{
			"OriginOS：i管家 → 應用管理 → 許可權 → 自啟動管理 → 允許 Termux",
			"OriginOS：設定 → 電池 → 背景高耗電管理 → 允許 Termux 背景執行",
		}, generic...)
	case OEMEMUI:
		return append([]string{
			"EMUI/HarmonyOS：設定 → 電池 → 應用啟動管理 → 關閉自動管理並手動允許 Termux",
			"EMUI/HarmonyOS：手機管家 → 加速清理 → 將 Termux 加入白名單",
		}, generic...)
	case OEMFlyme:
		return append([]string{
			"Flyme：設定 → 電池 → 允許 Termux 後台管理",
		}, generic...)
	default:
		return generic
	}
}

var (
	oemOnce   sync.Once
	oemCached *OEMInfo
)

// DetectOEM classifies the running device. On non-Android hosts it always
// returns an empty (unknown, non-aggressive) info. The result is memoised.
func DetectOEM() *OEMInfo {
	oemOnce.Do(func() {
		oemCached = classifyOEM(readProps())
	})
	return oemCached
}

// Note: there is deliberately no ResetOEMCache() helper. Mutating the global
// oemOnce / oemCached would be a data race under parallel tests; unit tests
// exercise the pure classifyOEM function with mock property maps instead.

// readProps collects the interesting system properties. On Android/Termux it
// shells out to getprop once and parses the whole dump; elsewhere it returns
// an empty map so classification yields "unknown".
func readProps() map[string]string {
	if runtime.GOOS != "android" && !Detect().IsTermux {
		return map[string]string{}
	}
	// Prefer the absolute path: daemons (crond, scheduled backups) or
	// minimal shells may run without /system/bin on PATH.
	cmdPath := "getprop"
	if _, err := os.Stat("/system/bin/getprop"); err == nil {
		cmdPath = "/system/bin/getprop"
	}
	// Bound the call: a hung binder/SELinux-blocked getprop (seen on
	// Android 14+) must not freeze the process indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cmdPath).Output()
	if err != nil {
		return map[string]string{}
	}
	return ParseGetprop(string(out))
}

// ParseGetprop parses the output of `getprop` ([key]: [value] lines) into a
// map. Exported so tests (and future tooling) can feed captured prop dumps.
func ParseGetprop(dump string) map[string]string {
	props := map[string]string{}
	for _, line := range strings.Split(dump, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") {
			continue
		}
		closeKey := strings.Index(line, "]:")
		if closeKey < 0 {
			continue
		}
		key := line[1:closeKey]
		val := strings.TrimSpace(line[closeKey+2:])
		// Strip only a single pair of outer brackets; values may legitimately
		// contain brackets themselves (e.g. display IDs like "[HyperOS]_V1").
		if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") && len(val) >= 2 {
			val = val[1 : len(val)-1]
		}
		props[key] = val
	}
	return props
}

// classifyOEM maps a property set to an OEMInfo. It is pure so it can be
// unit-tested with captured getprop dumps from real devices.
func classifyOEM(props map[string]string) *OEMInfo {
	info := &OEMInfo{
		Manufacturer: props["ro.product.manufacturer"], // raw casing, for display
		Brand:        props["ro.product.brand"],        // raw casing, for display
		DisplayID:    props["ro.build.display.id"],
	}

	has := func(k string) bool { return strings.TrimSpace(props[k]) != "" }
	containsFold := func(v string, subs ...string) bool {
		lv := strings.ToLower(v)
		for _, s := range subs {
			if strings.Contains(lv, strings.ToLower(s)) {
				return true
			}
		}
		return false
	}
	// Sub-brands (POCO, iQOO, realme, Redmi, Honor) frequently report the
	// parent company in one of manufacturer/brand, so match either field.
	isBrandOrMaker := func(targets ...string) bool {
		m := strings.ToLower(info.Manufacturer)
		b := strings.ToLower(info.Brand)
		for _, t := range targets {
			lt := strings.ToLower(t)
			if m == lt || b == lt {
				return true
			}
		}
		return false
	}

	switch {
	// ColorOS / OxygenOS / realme UI. ro.build.version.oplusrom is the key
	// Oplus-era property on ColorOS 11-15, Realme UI 2+ and OxygenOS 12+.
	case has("ro.build.version.oplusrom"), has("ro.build.version.opporom"),
		has("ro.build.version.realmeui"),
		isBrandOrMaker("oppo", "oneplus", "realme"):
		info.Family = OEMColorOS
	// HyperOS / MIUI. ro.mi.os.version.name is the dedicated HyperOS prop;
	// ro.miui.ui.version.name covers legacy MIUI devices.
	case has("ro.mi.os.version.name"), has("ro.miui.ui.version.name"),
		containsFold(info.DisplayID, "miui", "hyperos"),
		isBrandOrMaker("xiaomi", "redmi", "poco"):
		info.Family = OEMMIUI
	case has("ro.vivo.os.name"), has("ro.vivo.os.version"),
		isBrandOrMaker("vivo", "iqoo"):
		info.Family = OEMOriginOS
	// EMUI / HarmonyOS / MagicOS. hw_emui_api_level values are often plain
	// integers ("9", "11"), so presence alone is enough; ro.build.version.magic
	// covers Honor after its spin-off from Huawei.
	case has("ro.build.version.emui"), has("ro.huawei.build.display.id"),
		has("ro.build.hw_emui_api_level"), has("ro.build.version.magic"),
		isBrandOrMaker("huawei", "honor"):
		info.Family = OEMEMUI
	case isBrandOrMaker("meizu"), containsFold(info.DisplayID, "flyme"):
		info.Family = OEMFlyme
	default:
		// Known-manufacturer list of other aggressive killers without a
		// dedicated checklist yet (Samsung is comparatively benign and
		// intentionally NOT flagged here).
		aggressiveOthers := []string{"asus", "lenovo", "zte", "gionee", "letv", "coolpad"}
		for _, m := range aggressiveOthers {
			if isBrandOrMaker(m) {
				info.Family = OEMOther
				break
			}
		}
	}

	info.Aggressive = info.Family != OEMUnknown
	return info
}
