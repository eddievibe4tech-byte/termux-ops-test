// OEM ROM detection: aggressive battery managers (ColorOS, MIUI/HyperOS,
// OriginOS, EMUI, Flyme, …) ship "task killer" mechanisms that freeze or
// kill Termux despite wake locks, causing scheduled backups to silently
// not run (issue #43). This file probes well-known Android system properties
// via getprop (or by parsing a captured build.prop on non-Android hosts,
// which keeps the logic unit-testable) and classifies the device so that
// `hlops doctor` can show an OEM-specific survival checklist.
package termux

import (
	"os/exec"
	"runtime"
	"strings"
	"sync"
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

// The properties we rely on: ro.build.version.opporom / riscos.version cover
// ColorOS and realme UI; ro.miui.ui.version.name covers MIUI and HyperOS;
// vivo variants expose ro.vivo.os.name / ro.vivo.os.version; Huawei uses
// ro.build.version.emui / ro.build.hw_emui_api_level. classifyOEM below reads
// them from a full `getprop` dump.

// OEMInfo is the classification result for one device.
type OEMInfo struct {
	// Family is one of the OEM* constants above (OEMUnknown when no
	// aggressive task killer was detected).
	Family string
	// Manufacturer / Brand echo the raw props for display purposes.
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

// ResetOEMCache clears the memoised OEM classification (used by tests).
func ResetOEMCache() {
	oemCached = nil
	oemOnce = sync.Once{}
}

// readProps collects the interesting system properties. On Android/Termux it
// shells out to getprop once and parses the whole dump; elsewhere it returns
// an empty map so classification yields "unknown".
func readProps() map[string]string {
	if runtime.GOOS != "android" && !Detect().IsTermux {
		return map[string]string{}
	}
	out, err := exec.Command("getprop").Output()
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
		val = strings.Trim(val, "[]")
		props[key] = val
	}
	return props
}

// classifyOEM maps a property set to an OEMInfo. It is pure so it can be
// unit-tested with captured getprop dumps from real devices.
func classifyOEM(props map[string]string) *OEMInfo {
	info := &OEMInfo{
		Manufacturer: strings.ToLower(props["ro.product.manufacturer"]),
		Brand:        strings.ToLower(props["ro.product.brand"]),
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

	switch {
	case has("ro.build.version.opporom"), has("ro.build.version.riscoversion"),
		info.Manufacturer == "oppo", info.Manufacturer == "oneplus", info.Manufacturer == "realme":
		info.Family = OEMColorOS
	case has("ro.miui.ui.version.name"), containsFold(info.DisplayID, "miui", "hyperos"),
		info.Manufacturer == "xiaomi", info.Manufacturer == "redmi", info.Manufacturer == "poco":
		info.Family = OEMMIUI
	case has("ro.vivo.os.name"), has("ro.vivo.os.version"),
		info.Manufacturer == "vivo", info.Manufacturer == "iqoo":
		info.Family = OEMOriginOS
	case has("ro.build.version.emui"), has("ro.huawei.build.display.id"),
		strings.Contains(strings.ToLower(props["ro.build.hw_emui_api_level"]), "."),
		info.Manufacturer == "huawei", info.Manufacturer == "honor":
		info.Family = OEMEMUI
	case info.Manufacturer == "meizu", containsFold(info.DisplayID, "flyme"):
		info.Family = OEMFlyme
	default:
		// Known-manufacturer list of other aggressive killers without a
		// dedicated checklist yet (Samsung is comparatively benign and
		// intentionally NOT flagged here).
		aggressiveOthers := []string{"asus", "lenovo", "zte", "gionee", "letv", "coolpad"}
		for _, m := range aggressiveOthers {
			if info.Manufacturer == m {
				info.Family = OEMOther
				break
			}
		}
	}

	info.Aggressive = info.Family != OEMUnknown
	return info
}
