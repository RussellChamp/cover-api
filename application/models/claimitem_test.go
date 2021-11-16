package models

import (
	"fmt"
	"testing"

	"github.com/silinternational/cover-api/api"
	"github.com/silinternational/cover-api/domain"
)

func (ms *ModelSuite) TestClaimItem_Validate() {
	tests := []struct {
		name      string
		claimItem *ClaimItem
		errField  string
		wantErr   bool
	}{
		{
			name: "valid status, not approved",
			claimItem: &ClaimItem{
				Claim: Claim{
					IncidentType: api.ClaimIncidentTypePhysicalDamage,
				},
				PayoutOption: api.PayoutOptionRepair,
			},
			errField: "",
			wantErr:  false,
		},
		{
			name: "invalid payout option",
			claimItem: &ClaimItem{
				PayoutOption: api.PayoutOption("bitcoin"),
			},
			errField: "ClaimItem.PayoutOption",
			wantErr:  true,
		},
		{
			name: "invalid payout option for Evacuation",
			claimItem: &ClaimItem{
				Claim: Claim{
					Status:       api.ClaimStatusDraft,
					IncidentType: api.ClaimIncidentTypeEvacuation,
				},
				PayoutOption: api.PayoutOptionFMV,
			},
			errField: "ClaimItem.PayoutOption",
			wantErr:  true,
		},
		{
			name: "valid payout option for Evacuation",
			claimItem: &ClaimItem{
				Claim: Claim{
					IncidentType: api.ClaimIncidentTypeEvacuation,
				},
				PayoutOption: api.PayoutOptionFixedFraction,
			},
			wantErr: false,
		},
		{
			name: "invalid payout option for Theft",
			claimItem: &ClaimItem{
				Claim: Claim{
					Status:       api.ClaimStatusDraft,
					IncidentType: api.ClaimIncidentTypeTheft,
				},
				PayoutOption: api.PayoutOptionFixedFraction,
			},
			errField: "ClaimItem.PayoutOption",
			wantErr:  true,
		},
		{
			name: "valid payout option for Theft",
			claimItem: &ClaimItem{
				Claim: Claim{
					IncidentType: api.ClaimIncidentTypeTheft,
				},
				PayoutOption: api.PayoutOptionFMV,
			},
			wantErr: false,
		},
		{
			name: "invalid payout option for Impact",
			claimItem: &ClaimItem{
				Claim: Claim{
					Status:       api.ClaimStatusDraft,
					IncidentType: api.ClaimIncidentTypePhysicalDamage,
				},
				PayoutOption: api.PayoutOptionFixedFraction,
			},
			errField: "ClaimItem.PayoutOption",
			wantErr:  true,
		},
		{
			name: "valid payout option for Impact",
			claimItem: &ClaimItem{
				Claim: Claim{
					IncidentType: api.ClaimIncidentTypePhysicalDamage,
				},
				PayoutOption: api.PayoutOptionRepair,
			},
			wantErr: false,
		},
		{
			name: "valid status, approved",
			claimItem: &ClaimItem{
				PayoutOption: api.PayoutOptionRepair,
			},
			errField: "",
			wantErr:  false,
		},
	}
	mt := ms.T()
	for _, tt := range tests {
		mt.Run(tt.name, func(t *testing.T) {
			vErr, _ := tt.claimItem.Validate(ms.DB)
			if tt.wantErr {
				if vErr.Count() == 0 {
					mt.Fatal("Expected an error, but did not get one")
				} else if len(vErr.Get(tt.errField)) == 0 {
					mt.Fatalf("Expected an error on field %v, but got none (errors: %+v)", tt.errField, vErr.Errors)
				}
			} else if vErr.HasAny() {
				mt.Fatalf("Unexpected error: %+v", vErr)
			}
		})
	}
}

func (ms *ModelSuite) TestClaimItem_ValidateForSubmit() {
	good := ClaimItem{
		IsRepairable:    false,
		RepairEstimate:  100,
		ReplaceEstimate: 1000,
		PayoutOption:    api.PayoutOptionRepair,
		FMV:             1000,
		Claim:           Claim{IncidentType: api.ClaimIncidentTypeTheft},
	}

	missingPayoutOption := good
	missingPayoutOption.PayoutOption = ""

	theftIsNotRepairable := good
	theftIsNotRepairable.IsRepairable = true

	missingReplaceEstimate := good
	missingReplaceEstimate.PayoutOption = api.PayoutOptionReplacement
	missingReplaceEstimate.ReplaceEstimate = 0

	missingFMV := good
	missingFMV.PayoutOption = api.PayoutOptionFMV
	missingFMV.FMV = 0

	missingRepairEstimate := good
	missingRepairEstimate.IsRepairable = true
	missingRepairEstimate.Claim.IncidentType = api.ClaimIncidentTypePhysicalDamage
	missingRepairEstimate.RepairEstimate = 0

	missingImpactFMV := good
	missingImpactFMV.IsRepairable = true
	missingImpactFMV.Claim.IncidentType = api.ClaimIncidentTypePhysicalDamage
	missingImpactFMV.FMV = 0

	invalidPayoutOption := good
	invalidPayoutOption.IsRepairable = false
	invalidPayoutOption.Claim.IncidentType = api.ClaimIncidentTypePhysicalDamage
	invalidPayoutOption.PayoutOption = api.PayoutOptionRepair

	invalidPayoutOptionEvacuation := good
	invalidPayoutOptionEvacuation.Claim.IncidentType = api.ClaimIncidentTypeEvacuation
	invalidPayoutOptionEvacuation.PayoutOption = api.PayoutOptionRepair

	missingReplaceEstimateImpact := good
	missingReplaceEstimateImpact.Claim.IncidentType = api.ClaimIncidentTypePhysicalDamage
	missingReplaceEstimateImpact.PayoutOption = api.PayoutOptionReplacement
	missingReplaceEstimateImpact.ReplaceEstimate = 0

	missingFMVImpact := good
	missingFMVImpact.Claim.IncidentType = api.ClaimIncidentTypePhysicalDamage
	missingFMVImpact.PayoutOption = api.PayoutOptionFMV
	missingFMVImpact.FMV = 0

	tests := []struct {
		name      string
		claimItem ClaimItem
		want      api.ErrorKey
	}{
		{
			name:      "missing payout option",
			claimItem: missingPayoutOption,
			want:      api.ErrorClaimItemMissingPayoutOption,
		},
		{
			name:      "item is not repairable",
			claimItem: theftIsNotRepairable,
			want:      api.ErrorClaimItemNotRepairable,
		},
		{
			name:      "missing replace estimate",
			claimItem: missingReplaceEstimate,
			want:      api.ErrorClaimItemMissingReplaceEstimate,
		},
		{
			name:      "missing FMV",
			claimItem: missingFMV,
			want:      api.ErrorClaimItemMissingFMV,
		},
		{
			name:      "invalid payout option",
			claimItem: invalidPayoutOption,
			want:      api.ErrorClaimItemInvalidPayoutOption,
		},
		{
			name:      "invalid payout option, evacuation",
			claimItem: invalidPayoutOptionEvacuation,
			want:      api.ErrorClaimItemInvalidPayoutOption,
		},
		{
			name:      "missing repair estimate",
			claimItem: missingRepairEstimate,
			want:      api.ErrorClaimItemMissingRepairEstimate,
		},
		{
			name:      "missing impact FMV",
			claimItem: missingImpactFMV,
			want:      api.ErrorClaimItemMissingFMV,
		},
		{
			name:      "missing replace estimate (impact)",
			claimItem: missingReplaceEstimateImpact,
			want:      api.ErrorClaimItemMissingReplaceEstimate,
		},
		{
			name:      "missing FMV (impact)",
			claimItem: missingFMVImpact,
			want:      api.ErrorClaimItemMissingFMV,
		},
		{
			name:      "good",
			claimItem: good,
			want:      "",
		},
	}
	for _, tt := range tests {
		ms.T().Run(tt.name, func(t *testing.T) {
			if got := tt.claimItem.ValidateForSubmit(ms.DB); got != tt.want {
				t.Errorf("ValidateForSubmit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func (ms *ModelSuite) TestClaimItem_Compare() {
	fixtures := CreateItemFixtures(ms.DB, FixturesConfig{
		ClaimsPerPolicy:    1,
		ClaimItemsPerClaim: 1,
	})
	claim := fixtures.Claims[0]
	claim.LoadClaimItems(ms.DB, false)
	newCItem := claim.ClaimItems[0]

	oldCItem := ClaimItem{
		ItemID:          domain.GetUUID(),
		IsRepairable:    true,
		RepairEstimate:  1111,
		RepairActual:    1112,
		ReplaceEstimate: 2221,
		ReplaceActual:   2222,
		PayoutOption:    api.PayoutOptionReplacement,
		PayoutAmount:    3331,
		FMV:             4441,
		Country:         "Mali",
	}

	tests := []struct {
		name string
		new  ClaimItem
		old  ClaimItem
		want []FieldUpdate
	}{
		{
			name: "single test case",
			new:  newCItem,
			old:  oldCItem,
			want: []FieldUpdate{
				{
					FieldName: FieldClaimItemItemID,
					OldValue:  oldCItem.ItemID.String(),
					NewValue:  newCItem.ItemID.String(),
				},
				{
					FieldName: FieldClaimItemIsRepairable,
					OldValue:  fmt.Sprintf("%t", oldCItem.IsRepairable),
					NewValue:  fmt.Sprintf("%t", newCItem.IsRepairable),
				},
				{
					FieldName: FieldClaimItemRepairEstimate,
					OldValue:  api.Currency(oldCItem.RepairEstimate).String(),
					NewValue:  api.Currency(newCItem.RepairEstimate).String(),
				},
				{
					FieldName: FieldClaimItemRepairActual,
					OldValue:  api.Currency(oldCItem.RepairActual).String(),
					NewValue:  api.Currency(newCItem.RepairActual).String(),
				},
				{
					FieldName: FieldClaimItemReplaceEstimate,
					OldValue:  api.Currency(oldCItem.ReplaceEstimate).String(),
					NewValue:  api.Currency(newCItem.ReplaceEstimate).String(),
				},
				{
					FieldName: FieldClaimItemReplaceActual,
					OldValue:  api.Currency(oldCItem.ReplaceActual).String(),
					NewValue:  api.Currency(newCItem.ReplaceActual).String(),
				},
				{
					FieldName: FieldClaimItemPayoutOption,
					OldValue:  string(oldCItem.PayoutOption),
					NewValue:  string(newCItem.PayoutOption),
				},
				{
					FieldName: FieldClaimItemPayoutAmount,
					OldValue:  api.Currency(oldCItem.PayoutAmount).String(),
					NewValue:  api.Currency(newCItem.PayoutAmount).String(),
				},
				{
					FieldName: FieldClaimItemFMV,
					OldValue:  api.Currency(oldCItem.FMV).String(),
					NewValue:  api.Currency(newCItem.FMV).String(),
				},
				{
					FieldName: FieldClaimItemLocation,
					OldValue:  oldCItem.GetLocation().String(),
					NewValue:  newCItem.GetLocation().String(),
				},
			},
		},
	}

	for _, tt := range tests {
		ms.T().Run(tt.name, func(t *testing.T) {
			got := tt.new.Compare(tt.old)
			ms.ElementsMatch(tt.want, got)
		})
	}
}

func (ms *ModelSuite) TestClaimItem_updatePayoutAmount() {
	params := []UpdateClaimItemsParams{
		{PayoutOption: api.PayoutOptionRepair, RepairEstimate: 100},
		{PayoutOption: api.PayoutOptionReplacement, ReplaceEstimate: 200},
		{PayoutOption: api.PayoutOptionFMV, FMV: 300},
		{PayoutOption: api.PayoutOptionFixedFraction},
		{PayoutOption: api.PayoutOptionRepair, RepairEstimate: 1000},
		{PayoutOption: api.PayoutOptionRepair, RepairActual: 500},
	}

	fixtures := CreateItemFixtures(ms.DB, FixturesConfig{ClaimsPerPolicy: len(params), ClaimItemsPerClaim: 1})

	for i, p := range params {
		UpdateClaimItems(ms.DB, fixtures.Claims[i], p)
		fixtures.Items[i].CoverageAmount = 900
		ms.NoError(ms.DB.Update(&fixtures.Items[i]))
	}

	// for FixedFraction, the incident type must be Evacuation
	fixtures.Claims[3].IncidentType = api.ClaimIncidentTypeEvacuation
	ms.NoError(ms.DB.Update(&fixtures.Claims[3]))

	review3Claim := UpdateClaimStatus(ms.DB, fixtures.Claims[5], api.ClaimStatusReview3, "")

	testCtx := CreateTestContext(fixtures.Users[0])

	tests := []struct {
		name      string
		claimItem ClaimItem
		want      api.Currency
	}{
		{
			name:      "repair",
			claimItem: fixtures.Claims[0].ClaimItems[0],
			want:      95,
		},
		{
			name:      "replace",
			claimItem: fixtures.Claims[1].ClaimItems[0],
			want:      190,
		},
		{
			name:      "fmv",
			claimItem: fixtures.Claims[2].ClaimItems[0],
			want:      285,
		},
		{
			name:      "fixed-fraction",
			claimItem: fixtures.Claims[3].ClaimItems[0],
			want:      600,
		},
		{
			name:      "capped by CoverageAmount",
			claimItem: fixtures.Claims[4].ClaimItems[0],
			want:      855,
		},
		{
			name:      "use RepairActual",
			claimItem: review3Claim.ClaimItems[0],
			want:      475,
		},
	}
	for _, tt := range tests {
		ms.T().Run(tt.name, func(t *testing.T) {
			// Get a fresh copy of the claimItem to ensure the UUT hydrates it as necessary
			var claimItem ClaimItem
			ms.NoError(claimItem.FindByID(ms.DB, tt.claimItem.ID))

			err := claimItem.updatePayoutAmount(testCtx)
			ms.NoError(err)

			ms.Equal(tt.want, claimItem.PayoutAmount, "didn't get the correct PayoutAmount")
		})
	}
}
