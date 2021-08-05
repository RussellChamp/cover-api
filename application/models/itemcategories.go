package models

import (
	"time"

	"github.com/gobuffalo/pop/v5"
	"github.com/gobuffalo/validate/v3"
	"github.com/gofrs/uuid"

	"github.com/silinternational/riskman-api/api"
)

var ValidItemCategoryStatuses = map[api.ItemCategoryStatus]struct{}{
	api.ItemCategoryStatusDraft:      {},
	api.ItemCategoryStatusEnabled:    {},
	api.ItemCategoryStatusDeprecated: {},
	api.ItemCategoryStatusDisabled:   {},
}

// ItemCategories is a slice of ItemCategory objects
type ItemCategories []ItemCategory

// ItemCategory model
type ItemCategory struct {
	ID             uuid.UUID              `db:"id"`
	RiskCategoryID uuid.UUID              `db:"risk_category_id"`
	Name           string                 `db:"name" validate:"required"`
	HelpText       string                 `db:"help_text"`
	Status         api.ItemCategoryStatus `db:"status" validate:"itemCategoryStatus"`
	AutoApproveMax int                    `db:"auto_approve_max"`
	CreatedAt      time.Time              `db:"created_at"`
	UpdatedAt      time.Time              `db:"updated_at"`

	RiskCategory RiskCategory `belongs_to:"risk_categories" fk_id:"RiskCategoryID" validate:"-"`
}

// Validate gets run every time you call a "pop.Validate*" (pop.ValidateAndSave, pop.ValidateAndCreate, pop.ValidateAndUpdate) method.
func (r *ItemCategory) Validate(tx *pop.Connection) (*validate.Errors, error) {
	return validateModel(r), nil
}

func (r *ItemCategory) GetID() uuid.UUID {
	return r.ID
}

func (r *ItemCategory) FindByID(tx *pop.Connection, id uuid.UUID) error {
	return tx.Find(r, id)
}

func ConvertItemCategory(tx *pop.Connection, ic ItemCategory) (api.ItemCategory, error) {
	return api.ItemCategory{
		ID:             ic.ID,
		Name:           ic.Name,
		HelpText:       ic.HelpText,
		Status:         ic.Status,
		AutoApproveMax: ic.AutoApproveMax,
		CreatedAt:      ic.CreatedAt,
		UpdatedAt:      ic.UpdatedAt,
	}, nil
}

func ConvertItemCategories(tx *pop.Connection, ics ItemCategories) (api.ItemCategories, error) {
	cats := make(api.ItemCategories, len(ics))
	for i, ic := range ics {
		var err error
		cats[i], err = ConvertItemCategory(tx, ic)
		if err != nil {
			return api.ItemCategories{}, err
		}
	}
	return cats, nil
}
