package healthcheck

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthcheckCheck(t *testing.T) {

	testCases := []struct {
		name                  string
		wsrepState            string
		readOnly              string
		AvailableWhenDonor    bool
		AvailableWhenReadOnly bool
		expected              bool
		expectedMsg           string
	}{
		{"00-joining", "1", "OFF", false, false, false, "not synced"},
		{"00-joined", "3", "OFF", false, false, false, "not synced"},
		{"00-donor", "2", "OFF", false, false, false, "not synced"},
		{"11-donor", "2", "ON", true, true, true, "synced"},
		{"10-donor-ro", "2", "ON", true, false, false, "read-only"},
		{"10-donor-ok", "2", "OFF", true, false, true, "synced"},
		{"01-synced", "4", "ON", false, true, true, "synced"},
		{"00-synced-ro", "4", "ON", false, false, false, "read-only"},
		{"00-synced-ok", "4", "OFF", false, false, true, "synced"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)

			db, mock, err := sqlmock.New()
			require.NoError(t, err)

			mock.ExpectQuery("SHOW STATUS LIKE 'wsrep_local_state'").WillReturnRows(
				sqlmock.NewRows([]string{"Variable_name", "Value"}).AddRow("wsrep_local_state", tc.wsrepState),
			)
			mock.ExpectQuery("SHOW GLOBAL VARIABLES LIKE 'read_only'").WillReturnRows(
				sqlmock.NewRows([]string{"Variable_name", "Value"}).AddRow("read_only", tc.readOnly),
			)

			h := New(db)
			h.wasJoined = true

			healthy, msg, err := h.Check(tc.AvailableWhenDonor, tc.AvailableWhenReadOnly)
			assert.NoError(err)
			assert.Equal(tc.expected, healthy)
			assert.Equal(tc.expectedMsg, msg)
		})
	}
}
