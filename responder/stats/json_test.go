package stats

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_JSONStatsInvalidFormat(t *testing.T) {
	stats := JSONStats{}

	stats.IncInvalidFormat()
	assert.Equal(t, int64(1), stats.invalidFormat)
}

func Test_JSONStatsRequests(t *testing.T) {
	stats := JSONStats{}

	stats.IncRequests()
	assert.Equal(t, int64(1), stats.requests)
}

func Test_JSONStatsResponses(t *testing.T) {
	stats := JSONStats{}

	stats.IncResponses()
	assert.Equal(t, int64(1), stats.responses)
}

func Test_JSONStatsListeners(t *testing.T) {
	stats := JSONStats{}

	stats.IncListeners()
	assert.Equal(t, int64(1), stats.listeners)

	stats.DecListeners()
	assert.Equal(t, int64(0), stats.listeners)
}

func Test_JSONStatsWorkers(t *testing.T) {
	stats := JSONStats{}

	stats.IncWorkers()
	assert.Equal(t, int64(1), stats.workers)

	stats.DecWorkers()
	assert.Equal(t, int64(0), stats.workers)
}

func Test_JSONStatsAnnounce(t *testing.T) {
	stats := JSONStats{}

	stats.SetAnnounce()
	assert.Equal(t, int64(1), stats.announce)

	stats.ResetAnnounce()
	assert.Equal(t, int64(0), stats.announce)
}

func Test_JSONStatsSetPrefix(t *testing.T) {
	stats := JSONStats{}

	stats.SetPrefix("test")
	assert.Equal(t, "test", stats.prefix)
}

func Test_JSONStatsToMap(t *testing.T) {
	j := JSONStats{
		invalidFormat: 1,
		requests:      2,
		responses:     3,
		listeners:     4,
		workers:       5,
		announce:      6,
	}
	j.SetPrefix("test.")
	result := j.toMap()

	expectedMap := make(map[string]int64)
	expectedMap["test.invalidformat"] = 1
	expectedMap["test.requests"] = 2
	expectedMap["test.responses"] = 3
	expectedMap["test.listeners"] = 4
	expectedMap["test.workers"] = 5
	expectedMap["test.announce"] = 6

	assert.Equal(t, expectedMap, result)
}
