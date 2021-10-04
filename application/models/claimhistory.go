package models

import (
	"time"

	"github.com/gobuffalo/nulls"
	"github.com/gobuffalo/pop/v5"
	"github.com/gobuffalo/validate/v3"
	"github.com/gofrs/uuid"

	"github.com/silinternational/cover-api/api"
	"github.com/silinternational/cover-api/domain"
)

type ClaimHistories []ClaimHistory

type ClaimHistory struct {
	ID          uuid.UUID  `db:"id"`
	ClaimID     uuid.UUID  `db:"claim_id"`
	ClaimItemID nulls.UUID `db:"claim_item_id"`
	UserID      uuid.UUID  `db:"user_id"`
	Action      string     `db:"action"`
	FieldName   string     `db:"field_name"`
	OldValue    string     `db:"old_value"`
	NewValue    string     `db:"new_value"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

// Validate gets run every time you call a "pop.Validate*" (pop.ValidateAndSave, pop.ValidateAndCreate, pop.ValidateAndUpdate) method.
func (ch *ClaimHistory) Validate(tx *pop.Connection) (*validate.Errors, error) {
	return validateModel(ch), nil
}

func (ch *ClaimHistory) Create(tx *pop.Connection) error {
	return create(tx, ch)
}

func (ch *ClaimHistory) GetID() uuid.UUID {
	return ch.ID
}

func (ch *ClaimHistory) FindByID(tx *pop.Connection, id uuid.UUID) error {
	return tx.Find(ch, id)
}

// RecentClaimStatusChanges hydrates the ClaimHistories with those that
//  have been created in the last week and that also have
//  a field_name of Status and
//  an action of Update
func (ch *ClaimHistories) RecentClaimStatusChanges(tx *pop.Connection) error {
	now := time.Now().UTC()
	cutoffDate := now.Add(-1 * domain.DurationWeek)
	err := tx.Where("created_at > ?", cutoffDate).
		Where("field_name = ? AND action = ?", "Status", api.HistoryActionUpdate).All(ch)

	if err != nil {
		return appErrorFromDB(err, api.ErrorQueryFailure)
	}
	return nil
}
