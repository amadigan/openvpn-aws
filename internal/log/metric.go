package log

import (
	"time"
)

type Metric struct {
	StartTime time.Time
	EndTime   time.Time
}

func StartMetric() *Metric {
	return &Metric{StartTime: time.Now()}
}

func (m *Metric) Stop() {
	m.EndTime = time.Now()
}

func (m *Metric) String() string {
	if m.EndTime.IsZero() {
		return "started at " + m.StartTime.String()
	}

	duration := m.EndTime.Sub(m.StartTime)

	if duration.Hours() >= 1 {
		duration = duration.Round(time.Minute)
	} else if duration.Minutes() >= 1 {
		duration = duration.Round(time.Second)
	} else if duration.Seconds() >= 1 {
		duration = duration.Round(time.Millisecond * 100)
	} else if duration.Milliseconds() >= 10 {
		duration = duration.Round(time.Millisecond * 10)
	}

	return duration.String()
}
