// Tiny subset of time. Duration is an int64 alias; Sleep is a no-op on
// wasm32-wasip1 (one thread, no clock we want to block the runqueue on).
//
// Now() is not declared here -- it needs the real wall clock, which no Go
// source can read, so wasigoc's generator recognizes the call `time.Now()`
// specially (like os.Args) and emits a call into wasigo::time_now()
// (std::chrono::system_clock, portable across the host build and
// wasi-sdk) instead of looking for a Go-source body. That also means a
// *bare* `Now()` call from inside this package itself wouldn't be
// recognized (the special-case only matches the `time.Now()` selector
// form), so nothing here calls it internally.
//
// Time is always UTC: no timezone/location support, so Date/Hour/etc. are
// exactly the UTC calendar fields. The civil calendar math is Howard
// Hinnant's days_from_civil/civil_from_days (a well-known constant-time,
// no-lookup-table algorithm), turned into plain Go here.
package time

type Duration int64

const (
	Nanosecond  Duration = 1
	Microsecond          = 1000 * Nanosecond
	Millisecond          = 1000 * Microsecond
	Second               = 1000 * Millisecond
	Minute               = 60 * Second
	Hour                 = 60 * Minute
)

func Sleep(d Duration) {}

func DurationNanoseconds(d Duration) int64 {
	return int64(d)
}

func DurationMicroseconds(d Duration) int64 {
	return int64(d) / int64(Microsecond)
}

func DurationMilliseconds(d Duration) int64 {
	return int64(d) / int64(Millisecond)
}

func DurationSeconds(d Duration) float64 {
	return float64(d) / float64(Second)
}

func DurationMinutes(d Duration) float64 {
	return float64(d) / float64(Minute)
}

func DurationHours(d Duration) float64 {
	return float64(d) / float64(Hour)
}

func padFrac(ns int64, width int) string {
	s := itoa64(ns)
	for len(s) < width {
		s = "0" + s
	}
	end := len(s)
	for end > 0 && s[end-1:end] == "0" {
		end--
	}
	return s[0:end]
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var rev []byte
	for n > 0 {
		rev = append(rev, byte(48+n%10))
		n = n / 10
	}
	out := make([]byte, len(rev))
	for i := 0; i < len(rev); i++ {
		out[i] = rev[len(rev)-1-i]
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func FormatDuration(d Duration) string {
	if d == 0 {
		return "0s"
	}
	neg := d < 0
	n := int64(d)
	if neg {
		n = -n
	}
	var out string
	if n < int64(Microsecond) {
		out = itoa64(n) + "ns"
	} else if n < int64(Millisecond) {
		whole := n / int64(Microsecond)
		frac := padFrac(n%int64(Microsecond)*1000, 3)
		out = itoa64(whole)
		if frac != "" {
			out = out + "." + frac
		}
		out = out + "µs"
	} else if n < int64(Second) {
		whole := n / int64(Millisecond)
		frac := padFrac(n%int64(Millisecond)*1000, 3)
		out = itoa64(whole)
		if frac != "" {
			out = out + "." + frac
		}
		out = out + "ms"
	} else {
		totalSec := n / int64(Second)
		fracNs := n % int64(Second)
		h := totalSec / 3600
		m := (totalSec % 3600) / 60
		s := totalSec % 60
		out = ""
		if h > 0 {
			out = out + itoa64(h) + "h"
		}
		if h > 0 || m > 0 {
			out = out + itoa64(m) + "m"
		}
		secStr := itoa64(s)
		frac := padFrac(fracNs, 9)
		if frac != "" {
			secStr = secStr + "." + frac
		}
		out = out + secStr + "s"
	}
	if neg {
		return "-" + out
	}
	return out
}

// Time is UTC-only: sec is a Unix second count, nsec (0..999999999) is the
// remainder.
type Time struct {
	sec  int64
	nsec int64
}

func Unix(sec int64, nsec int64) Time {
	extra := nsec / 1000000000
	rem := nsec % 1000000000
	if rem < 0 {
		rem = rem + 1000000000
		extra = extra - 1
	}
	return Time{sec: sec + extra, nsec: rem}
}

func (t Time) Unix() int64 {
	return t.sec
}

func (t Time) UnixNano() int64 {
	return t.sec*1000000000 + t.nsec
}

func (t Time) UnixMilli() int64 {
	return t.sec*1000 + t.nsec/1000000
}

func (t Time) Nanosecond() int {
	return int(t.nsec)
}

func (t Time) Add(d Duration) Time {
	total := t.sec*1000000000 + t.nsec + int64(d)
	sec := total / 1000000000
	nsec := total % 1000000000
	if nsec < 0 {
		nsec = nsec + 1000000000
		sec = sec - 1
	}
	return Time{sec: sec, nsec: nsec}
}

func (t Time) Sub(u Time) Duration {
	return Duration((t.sec-u.sec)*1000000000 + (t.nsec - u.nsec))
}

func (t Time) Before(u Time) bool {
	if t.sec != u.sec {
		return t.sec < u.sec
	}
	return t.nsec < u.nsec
}

func (t Time) After(u Time) bool {
	return u.Before(t)
}

func (t Time) Equal(u Time) bool {
	return t.sec == u.sec && t.nsec == u.nsec
}

func (t Time) IsZero() bool {
	return t.sec == 0 && t.nsec == 0
}

func floorDiv(a int64, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func floorMod(a int64, b int64) int64 {
	return a - floorDiv(a, b)*b
}

// civilFromDays: days since 1970-01-01 -> (year, month 1-12, day 1-31).
func civilFromDays(z int64) (int64, int64, int64) {
	z = z + 719468
	era := floorDiv(z, 146097)
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d := doy - (153*mp+2)/5 + 1
	var m int64
	if mp < 10 {
		m = mp + 3
	} else {
		m = mp - 9
	}
	if m <= 2 {
		y = y + 1
	}
	return y, m, d
}

func (t Time) days() int64 {
	return floorDiv(t.sec, 86400)
}

func (t Time) daySeconds() int64 {
	return floorMod(t.sec, 86400)
}

// Date returns the (year, month 1-12, day 1-31) UTC calendar date.
func (t Time) Date() (int, int, int) {
	y, m, d := civilFromDays(t.days())
	return int(y), int(m), int(d)
}

func (t Time) Year() int {
	y, _, _ := t.Date()
	return y
}

func (t Time) Month() int {
	_, m, _ := t.Date()
	return m
}

func (t Time) Day() int {
	_, _, d := t.Date()
	return d
}

func (t Time) Hour() int {
	return int(t.daySeconds() / 3600)
}

func (t Time) Minute() int {
	return int((t.daySeconds() % 3600) / 60)
}

func (t Time) Second() int {
	return int(t.daySeconds() % 60)
}

// Weekday: 0 = Sunday .. 6 = Saturday (matches real Go's time.Weekday
// iota order), computed off the fact 1970-01-01 was a Thursday (4).
func (t Time) Weekday() int {
	return int(floorMod(t.days()+4, 7))
}

func pad2(n int) string {
	s := itoa64(int64(n))
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}

// String: a fixed "YYYY-MM-DD HH:MM:SS UTC" -- real Go's reference-layout
// Format is not implemented (see README's stdlib tracker).
func (t Time) String() string {
	y, m, d := t.Date()
	return itoa64(int64(y)) + "-" + pad2(m) + "-" + pad2(d) + " " +
		pad2(t.Hour()) + ":" + pad2(t.Minute()) + ":" + pad2(t.Second()) + " UTC"
}
