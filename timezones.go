package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// tz is the active location used to derive "today" and all date math.
// It defaults to the machine's local zone and is replaced at startup by
// applyTimezone from the saved setting (default Asia/Manila).
var tz = time.Local

// applyTimezone sets the global tz from an IANA name, falling back to
// Asia/Manila and then the machine local zone if the name can't be loaded.
func applyTimezone(name string) {
	if loc, err := time.LoadLocation(name); err == nil {
		tz = loc
		return
	}
	if loc, err := time.LoadLocation("Asia/Manila"); err == nil {
		tz = loc
		return
	}
	tz = time.Local
}

// tzOffsetLabel returns a short current UTC offset like "UTC+8" for a zone.
func tzOffsetLabel(name string) string {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return ""
	}
	_, off := time.Now().In(loc).Zone()
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	h, mn := off/3600, (off%3600)/60
	if mn == 0 {
		return fmt.Sprintf("UTC%s%d", sign, h)
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, h, mn)
}

// tzIndexOf returns the position of name in zones, or 0 if not found.
func tzIndexOf(zones []string, name string) int {
	for i, z := range zones {
		if z == name {
			return i
		}
	}
	return 0
}

// availableTimezones returns the sorted, de-duplicated list of IANA zone
// names: whatever the OS ships under zoneinfo, unioned with a curated
// fallback so the picker is never empty (e.g. on Windows or in a sandbox).
func availableTimezones() []string {
	seen := map[string]bool{}
	var out []string
	add := func(z string) {
		if z == "" || seen[z] {
			return
		}
		seen[z] = true
		out = append(out, z)
	}
	for _, z := range systemZones() {
		add(z)
	}
	for _, z := range commonZones {
		add(z)
	}
	sort.Strings(out)
	return out
}

// systemZones walks the OS zoneinfo database and returns the zone names it
// finds (e.g. "Asia/Manila"). Returns nil if no database is present.
func systemZones() []string {
	dirs := []string{
		"/usr/share/zoneinfo",
		"/usr/share/lib/zoneinfo",
		"/usr/lib/zoneinfo",
		"/etc/zoneinfo",
	}
	var out []string
	for _, base := range dirs {
		_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				// Skip duplicate/legacy trees that clutter the list.
				if p != base {
					switch d.Name() {
					case "posix", "right", "SystemV", "Etc":
						return fs.SkipDir
					}
				}
				return nil
			}
			rel, err := filepath.Rel(base, p)
			if err != nil {
				return nil
			}
			if looksLikeZone(rel) {
				out = append(out, filepath.ToSlash(rel))
			}
			return nil
		})
		if len(out) > 0 {
			break // first database that exists wins
		}
	}
	return out
}

// looksLikeZone keeps Region/City entries (which start with an uppercase
// letter) and drops metadata files like zone.tab, +VERSION, leapseconds.
func looksLikeZone(rel string) bool {
	if rel == "" || rel[0] < 'A' || rel[0] > 'Z' {
		return false
	}
	base := filepath.Base(rel)
	if base == "Factory" {
		return false
	}
	return !strings.ContainsAny(base, ".+")
}

// commonZones is a curated fallback covering every region and UTC offset,
// used when the OS database is unavailable and to guarantee staples exist.
var commonZones = []string{
	"UTC",
	"Africa/Accra", "Africa/Addis_Ababa", "Africa/Algiers", "Africa/Cairo",
	"Africa/Casablanca", "Africa/Johannesburg", "Africa/Lagos", "Africa/Nairobi",
	"America/Anchorage", "America/Argentina/Buenos_Aires", "America/Bogota",
	"America/Chicago", "America/Denver", "America/Halifax", "America/Lima",
	"America/Los_Angeles", "America/Mexico_City", "America/New_York",
	"America/Phoenix", "America/Santiago", "America/Sao_Paulo", "America/Toronto",
	"America/Vancouver",
	"Asia/Almaty", "Asia/Baghdad", "Asia/Bangkok", "Asia/Beirut", "Asia/Colombo",
	"Asia/Dhaka", "Asia/Dubai", "Asia/Ho_Chi_Minh", "Asia/Hong_Kong",
	"Asia/Jakarta", "Asia/Jerusalem", "Asia/Karachi", "Asia/Kathmandu",
	"Asia/Kolkata", "Asia/Kuala_Lumpur", "Asia/Manila", "Asia/Riyadh",
	"Asia/Seoul", "Asia/Shanghai", "Asia/Singapore", "Asia/Taipei",
	"Asia/Tashkent", "Asia/Tehran", "Asia/Tokyo", "Asia/Yangon",
	"Atlantic/Azores", "Atlantic/Reykjavik",
	"Australia/Adelaide", "Australia/Brisbane", "Australia/Darwin",
	"Australia/Melbourne", "Australia/Perth", "Australia/Sydney",
	"Europe/Amsterdam", "Europe/Athens", "Europe/Berlin", "Europe/Brussels",
	"Europe/Bucharest", "Europe/Copenhagen", "Europe/Dublin", "Europe/Helsinki",
	"Europe/Istanbul", "Europe/Kyiv", "Europe/Lisbon", "Europe/London",
	"Europe/Madrid", "Europe/Moscow", "Europe/Oslo", "Europe/Paris",
	"Europe/Prague", "Europe/Rome", "Europe/Stockholm", "Europe/Vienna",
	"Europe/Warsaw", "Europe/Zurich",
	"Pacific/Auckland", "Pacific/Fiji", "Pacific/Guam", "Pacific/Honolulu",
	"Pacific/Port_Moresby",
}
