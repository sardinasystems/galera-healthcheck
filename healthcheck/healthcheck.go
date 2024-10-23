package healthcheck

import (
	"database/sql"
)

const (
	JOINING_STATE        = "1"
	DONOR_DESYNCED_STATE = "2"
	JOINED_STATE         = "3"
	SYNCED_STATE         = "4"
)

type Healthchecker struct {
	db *sql.DB

	wasJoined bool
	oldState  string
}

func New(db *sql.DB) *Healthchecker {
	return &Healthchecker{
		db:        db,
		wasJoined: false,
		oldState:  "0",
	}
}

func (h *Healthchecker) Check(AvailableWhenDonor, AvailableWhenReadOnly bool) (bool, string, error) {
	var variableName string
	var state string

	err := h.db.QueryRow("SHOW STATUS LIKE 'wsrep_local_state'").Scan(&variableName, &state)
	if err != nil {
		return false, "", err
	}

	var res, msg = false, "not synced"

	switch {
	case state != SYNCED_STATE && !h.wasJoined:
		if h.oldState == JOINED_STATE && state != JOINED_STATE {
			res, msg = false, "no synced"
			h.wasJoined = true
		} else {
			res, msg = false, "syncing"
		}

	case state == SYNCED_STATE || (state == DONOR_DESYNCED_STATE && AvailableWhenDonor):
		h.wasJoined = true
		res, msg = true, "synced"
		if !AvailableWhenReadOnly {
			var roValue string
			err = h.db.QueryRow("SHOW GLOBAL VARIABLES LIKE 'read_only'").Scan(&variableName, &roValue)
			if err != nil {
				return false, "", err
			}

			switch roValue {
			case "ON":
				res, msg = false, "read-only"
			}
		}
	}

	h.oldState = state
	return res, msg, nil
}
