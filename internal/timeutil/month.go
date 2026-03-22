package timeutil

import "time"

const MonthLayout = "2006-01"

func ParseMonth(value string) (time.Time, error) {
	return time.Parse(MonthLayout, value)
}

func FormatMonth(value time.Time) string {
	return value.UTC().Format(MonthLayout)
}

func AddMonths(value string, delta int) (string, error) {
	parsed, err := ParseMonth(value)
	if err != nil {
		return "", err
	}

	return FormatMonth(parsed.AddDate(0, delta, 0)), nil
}

func InRange(target, start, end string) bool {
	if target < start {
		return false
	}
	if end == "" {
		return true
	}
	return target <= end
}
