package models

import (
	"errors"
	"testing"
	"time"

	"github.com/gobuffalo/nulls"
	"github.com/gofrs/uuid"

	"github.com/silinternational/cover-api/api"
	"github.com/silinternational/cover-api/domain"
)

func (ms *ModelSuite) TestClaim_Validate() {
	tests := []struct {
		name     string
		claim    *Claim
		errField string
		wantErr  bool
	}{
		{
			name:     "empty struct",
			claim:    &Claim{},
			errField: "Claim.Status",
			wantErr:  true,
		},
		{
			name: "empty revision message - status = Revision",
			claim: &Claim{
				ReferenceNumber:     domain.RandomString(ClaimReferenceNumberLength, ""),
				PolicyID:            domain.GetUUID(),
				IncidentType:        api.ClaimIncidentTypePhysicalDamage,
				IncidentDate:        time.Now(),
				IncidentDescription: "testing123",
				Status:              api.ClaimStatusRevision,
			},
			errField: "Claim.StatusReason",
			wantErr:  true,
		},
		{
			name: "empty revision message - status = Denied",
			claim: &Claim{
				ReferenceNumber:     domain.RandomString(ClaimReferenceNumberLength, ""),
				PolicyID:            domain.GetUUID(),
				IncidentType:        api.ClaimIncidentTypePhysicalDamage,
				IncidentDate:        time.Now(),
				IncidentDescription: "testing123",
				Status:              api.ClaimStatusDenied,
			},
			errField: "Claim.StatusReason",
			wantErr:  true,
		},
		{
			name: "valid status",
			claim: &Claim{
				ReferenceNumber:     domain.RandomString(ClaimReferenceNumberLength, ""),
				PolicyID:            domain.GetUUID(),
				IncidentType:        api.ClaimIncidentTypePhysicalDamage,
				IncidentDate:        time.Now(),
				IncidentDescription: "testing123",
				Status:              api.ClaimStatusReview1,
			},
			errField: "",
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		ms.T().Run(tt.name, func(t *testing.T) {
			vErr, _ := tt.claim.Validate(DB)
			if tt.wantErr {
				if vErr.Count() == 0 {
					t.Errorf("Expected an error, but did not get one")
				} else if len(vErr.Get(tt.errField)) == 0 {
					t.Errorf("Expected an error on field %v, but got none (errors: %+v)", tt.errField, vErr.Errors)
				}
			} else if vErr.HasAny() {
				t.Errorf("Unexpected error: %+v", vErr)
			}
		})
	}
}

func (ms *ModelSuite) TestClaim_ReferenceNumber() {
	fixtures := CreatePolicyFixtures(ms.DB, FixturesConfig{
		NumberOfPolicies: 1,
	})
	claim := &Claim{
		PolicyID:            fixtures.Policies[0].ID,
		IncidentDate:        time.Now().UTC(),
		IncidentType:        api.ClaimIncidentTypePhysicalDamage,
		IncidentDescription: "fell",
		Status:              api.ClaimStatusReview1,
	}
	ms.NoError(claim.Create(ms.DB))
	ms.Len(claim.ReferenceNumber, ClaimReferenceNumberLength)
}

func (ms *ModelSuite) TestClaim_SubmitForApproval() {
	t := ms.T()

	fixConfig := FixturesConfig{
		NumberOfPolicies:    2,
		UsersPerPolicy:      2,
		DependentsPerPolicy: 2,
		ItemsPerPolicy:      4,
		ClaimsPerPolicy:     5,
		ClaimItemsPerClaim:  1,
	}

	fixtures := CreateItemFixtures(ms.DB, fixConfig)
	policy := fixtures.Policies[0]
	draftClaim := policy.Claims[0]
	revisionClaim := UpdateClaimStatus(ms.DB, policy.Claims[1], api.ClaimStatusRevision, "")
	reviewClaim := UpdateClaimStatus(ms.DB, policy.Claims[2], api.ClaimStatusReview1, "")
	emptyClaim := UpdateClaimStatus(ms.DB, policy.Claims[3], api.ClaimStatusDraft, "")
	itemNotReadyClaim := policy.Claims[4]

	goodParams := UpdateClaimItemsParams{
		PayoutOption:   api.PayoutOptionRepair,
		IsRepairable:   true,
		RepairEstimate: 1000,
		FMV:            2000,
	}
	UpdateClaimItems(ms.DB, draftClaim, goodParams)
	UpdateClaimItems(ms.DB, revisionClaim, goodParams)
	UpdateClaimItems(ms.DB, reviewClaim, goodParams)

	badParams := goodParams
	badParams.RepairEstimate = 0
	UpdateClaimItems(ms.DB, itemNotReadyClaim, badParams)

	tempClaim := emptyClaim
	tempClaim.LoadClaimItems(ms.DB, false)
	ms.NoError(ms.DB.Destroy(&tempClaim.ClaimItems[0]),
		"error trying to destroy ClaimItem fixture for test")

	tests := []struct {
		name            string
		claim           Claim
		wantErrContains string
		wantErrKey      api.ErrorKey
		wantErrCat      api.ErrorCategory
		wantStatus      api.ClaimStatus
	}{
		{
			name:            "bad start status",
			claim:           reviewClaim,
			wantErrKey:      api.ErrorClaimStatus,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "invalid claim status for submit",
		},
		{
			name:            "claim with no ClaimItem",
			claim:           emptyClaim,
			wantErrKey:      api.ErrorClaimMissingClaimItem,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "claim must have a claimItem if no longer in draft",
		},
		{
			name:            "from draft to review1, item not ready",
			claim:           itemNotReadyClaim,
			wantErrKey:      api.ErrorClaimItemMissingRepairEstimate,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "not valid for claim submission",
		},
		{
			name:       "from draft to review1",
			claim:      draftClaim,
			wantStatus: api.ClaimStatusReview1,
		},
		{
			name:       "from revision to review1",
			claim:      revisionClaim,
			wantStatus: api.ClaimStatusReview1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := CreateTestContext(fixtures.Users[0])
			got := tt.claim.SubmitForApproval(ctx)

			if tt.wantErrContains != "" {
				ms.Error(got, " did not return expected error")
				var appErr *api.AppError
				ms.True(errors.As(got, &appErr), "returned an error that is not an AppError")
				ms.Contains(got.Error(), tt.wantErrContains, "error message is not correct")
				ms.Equal(appErr.Key, tt.wantErrKey, "error key is not correct")
				ms.Equal(appErr.Category, tt.wantErrCat, "error category is not correct")
				return
			}
			ms.NoError(got)

			ms.Equal(tt.wantStatus, tt.claim.Status, "incorrect status")
			ms.Greater(tt.claim.TotalPayout, 0, "total payout was not set")
		})
	}
}

func (ms *ModelSuite) TestClaim_RequestRevision() {
	t := ms.T()

	fixConfig := FixturesConfig{
		NumberOfPolicies:    2,
		UsersPerPolicy:      2,
		DependentsPerPolicy: 2,
		ItemsPerPolicy:      4,
		ClaimsPerPolicy:     4,
		ClaimItemsPerClaim:  1,
	}

	fixtures := CreateItemFixtures(ms.DB, fixConfig)
	policy := fixtures.Policies[0]
	draftClaim := policy.Claims[0]
	review1Claim := UpdateClaimStatus(ms.DB, policy.Claims[2], api.ClaimStatusReview1, "")
	review3Claim := UpdateClaimStatus(ms.DB, policy.Claims[2], api.ClaimStatusReview3, "")
	emptyClaim := UpdateClaimStatus(ms.DB, policy.Claims[3], api.ClaimStatusReview1, "")

	tempClaim := emptyClaim
	tempClaim.LoadClaimItems(ms.DB, false)
	ms.NoError(ms.DB.Destroy(&tempClaim.ClaimItems[0]),
		"error trying to destroy ClaimItem fixture for test")

	admin := CreateAdminUsers(ms.DB)[AppRoleSteward]

	tests := []struct {
		name            string
		claim           Claim
		wantErrContains string
		wantErrKey      api.ErrorKey
		wantErrCat      api.ErrorCategory
		wantStatus      api.ClaimStatus
	}{
		{
			name:            "bad start status",
			claim:           draftClaim,
			wantErrKey:      api.ErrorValidation,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "invalid claim status transition from Draft to Revision",
		},
		{
			name:            "claim with no ClaimItem",
			claim:           emptyClaim,
			wantErrKey:      api.ErrorClaimMissingClaimItem,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "claim must have a claimItem if no longer in draft",
		},
		{
			name:       "from review1 to revision",
			claim:      review1Claim,
			wantStatus: api.ClaimStatusRevision,
		},
		{
			name:       "from review3 to revision",
			claim:      review3Claim,
			wantStatus: api.ClaimStatusRevision,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const message = "change all the things"
			ctx := CreateTestContext(admin)
			got := tt.claim.RequestRevision(ctx, message)

			if tt.wantErrContains != "" {
				ms.Error(got, " did not return expected error")
				var appErr *api.AppError
				ms.True(errors.As(got, &appErr), "returned an error that is not an AppError")
				ms.Contains(got.Error(), tt.wantErrContains, "error message is not correct")
				ms.Equal(tt.wantErrKey, appErr.Key, "error key is not correct")
				ms.Equal(tt.wantErrCat, appErr.Category, "error category is not correct")
				return
			}
			ms.NoError(got)

			ms.Equal(tt.wantStatus, tt.claim.Status, "incorrect status")
			ms.Equal(message, tt.claim.StatusReason, "incorrect status reason message")
		})
	}
}

func (ms *ModelSuite) TestClaim_Preapprove() {
	t := ms.T()

	fixConfig := FixturesConfig{
		NumberOfPolicies:    2,
		UsersPerPolicy:      2,
		DependentsPerPolicy: 2,
		ItemsPerPolicy:      4,
		ClaimsPerPolicy:     4,
		ClaimItemsPerClaim:  1,
	}

	fixtures := CreateItemFixtures(ms.DB, fixConfig)
	policy := fixtures.Policies[0]
	draftClaim := policy.Claims[0]
	review1Claim := UpdateClaimStatus(ms.DB, policy.Claims[2], api.ClaimStatusReview1, "")
	emptyClaim := UpdateClaimStatus(ms.DB, policy.Claims[3], api.ClaimStatusReview1, "")

	tempClaim := emptyClaim
	tempClaim.LoadClaimItems(ms.DB, false)
	ms.NoError(ms.DB.Destroy(&tempClaim.ClaimItems[0]),
		"error trying to destroy ClaimItem fixture for test")

	admin := CreateAdminUsers(ms.DB)[AppRoleSteward]

	tests := []struct {
		name            string
		claim           Claim
		wantErrContains string
		wantErrKey      api.ErrorKey
		wantErrCat      api.ErrorCategory
		wantStatus      api.ClaimStatus
	}{
		{
			name:            "bad start status",
			claim:           draftClaim,
			wantErrKey:      api.ErrorClaimStatus,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "invalid claim status for request receipt",
		},
		{
			name:            "claim with no ClaimItem",
			claim:           emptyClaim,
			wantErrKey:      api.ErrorClaimMissingClaimItem,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "claim must have a claimItem if no longer in draft",
		},
		{
			name:       "from review1 to receipt",
			claim:      review1Claim,
			wantStatus: api.ClaimStatusReceipt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := CreateTestContext(admin)
			got := tt.claim.RequestReceipt(ctx, "")

			if tt.wantErrContains != "" {
				ms.Error(got, " did not return expected error")
				var appErr *api.AppError
				ms.True(errors.As(got, &appErr), "returned an error that is not an AppError")
				ms.Contains(got.Error(), tt.wantErrContains, "error message is not correct")
				ms.Equal(appErr.Key, tt.wantErrKey, "error key is not correct")
				ms.Equal(appErr.Category, tt.wantErrCat, "error category is not correct")
				return
			}
			ms.NoError(got)

			ms.Equal(tt.wantStatus, tt.claim.Status, "incorrect status")
		})
	}
}

func (ms *ModelSuite) TestClaim_Approve() {
	t := ms.T()

	fixConfig := FixturesConfig{
		NumberOfPolicies:    2,
		UsersPerPolicy:      2,
		DependentsPerPolicy: 2,
		ItemsPerPolicy:      4,
		ClaimsPerPolicy:     6,
		ClaimItemsPerClaim:  1,
	}

	fixtures := CreateItemFixtures(ms.DB, fixConfig)

	adminUsers := CreateAdminUsers(ms.DB)
	steward := adminUsers[AppRoleSteward]
	signator := adminUsers[AppRoleSignator]

	policy := fixtures.Policies[0]
	draftClaim := policy.Claims[0]

	// Make one of the claims requesting an FMV payout
	fmvClaim := UpdateClaimStatus(ms.DB, policy.Claims[1], api.ClaimStatusReview1, "")
	fmvParams := UpdateClaimItemsParams{
		PayoutOption: api.PayoutOptionFMV,
		FMV:          2000,
	}
	UpdateClaimItems(ms.DB, fmvClaim, fmvParams)

	// Fail from Review1 to Review3 with wrong Payout Option
	notFMVClaim := UpdateClaimStatus(ms.DB, policy.Claims[5], api.ClaimStatusReview1, "")

	review2Claim := UpdateClaimStatus(ms.DB, policy.Claims[2], api.ClaimStatusReview2, "")

	policy.Claims[3].ReviewerID = nulls.NewUUID(steward.ID)
	review3Claim := UpdateClaimStatus(ms.DB, policy.Claims[3], api.ClaimStatusReview3, "")

	emptyClaim := UpdateClaimStatus(ms.DB, policy.Claims[4], api.ClaimStatusReview2, "")

	tempClaim := emptyClaim
	tempClaim.LoadClaimItems(ms.DB, false)
	ms.NoError(ms.DB.Destroy(&tempClaim.ClaimItems[0]),
		"error trying to destroy ClaimItem fixture for test")

	tests := []struct {
		name            string
		claim           Claim
		actor           User
		wantErrContains string
		wantErrKey      api.ErrorKey
		wantErrCat      api.ErrorCategory
		wantStatus      api.ClaimStatus
	}{
		{
			name:            "bad start status",
			claim:           draftClaim,
			actor:           steward,
			wantErrKey:      api.ErrorClaimStatus,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "invalid claim status for approve",
		},
		{
			name:            "claim with no ClaimItem",
			claim:           emptyClaim,
			actor:           steward,
			wantErrKey:      api.ErrorClaimMissingClaimItem,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "claim must have a claimItem if no longer in draft",
		},
		{
			name:            "not FMV from review1 to review3",
			actor:           steward,
			claim:           notFMVClaim,
			wantErrKey:      api.ErrorClaimItemInvalidPayoutOption,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "cannot approve payout option Repair from status Review1",
		},
		{
			name:       "from review1 to review3",
			actor:      steward,
			claim:      fmvClaim,
			wantStatus: api.ClaimStatusReview3,
		},
		{
			name:       "from review2 to review3",
			actor:      steward,
			claim:      review2Claim,
			wantStatus: api.ClaimStatusReview3,
		},
		{
			name:            "from review3 to approved, same user",
			actor:           steward,
			claim:           review3Claim,
			wantErrKey:      api.ErrorClaimInvalidApprover,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "different approver required for final approval",
		},
		{
			name:       "from review3 to approved, new user",
			actor:      signator,
			claim:      review3Claim,
			wantStatus: api.ClaimStatusApproved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := CreateTestContext(tt.actor)
			got := tt.claim.Approve(ctx)

			if tt.wantErrContains != "" {
				ms.Error(got, " did not return expected error")
				var appErr *api.AppError
				ms.True(errors.As(got, &appErr), "returned an error that is not an AppError")
				ms.Contains(got.Error(), tt.wantErrContains, "error message is not correct")
				ms.Equal(appErr.Key, tt.wantErrKey, "error key is not correct")
				ms.Equal(appErr.Category, tt.wantErrCat, "error category is not correct")
				return
			}
			ms.NoError(got)

			ms.Equal(tt.wantStatus, tt.claim.Status, "incorrect status")
			ms.Equal(tt.actor.ID.String(), tt.claim.ReviewerID.UUID.String(), "incorrect reviewer id")
			ms.WithinDuration(time.Now().UTC(), tt.claim.ReviewDate.Time, time.Second*2, "incorrect reviewer date id")
			ms.Equal("", tt.claim.StatusReason, "StatusReason should be empty after approval")
		})
	}
}

func (ms *ModelSuite) TestClaim_Deny() {
	t := ms.T()

	fixConfig := FixturesConfig{
		NumberOfPolicies:    2,
		UsersPerPolicy:      2,
		DependentsPerPolicy: 2,
		ItemsPerPolicy:      4,
		ClaimsPerPolicy:     5,
		ClaimItemsPerClaim:  1,
	}

	fixtures := CreateItemFixtures(ms.DB, fixConfig)

	admin := CreateAdminUsers(ms.DB)[AppRoleSteward]

	policy := fixtures.Policies[0]
	draftClaim := policy.Claims[0]
	review1Claim := UpdateClaimStatus(ms.DB, policy.Claims[1], api.ClaimStatusReview1, "")
	review2Claim := UpdateClaimStatus(ms.DB, policy.Claims[2], api.ClaimStatusReview2, "")
	review3Claim := UpdateClaimStatus(ms.DB, policy.Claims[3], api.ClaimStatusReview3, "")
	emptyClaim := UpdateClaimStatus(ms.DB, policy.Claims[4], api.ClaimStatusReview1, "")

	tempClaim := emptyClaim
	tempClaim.LoadClaimItems(ms.DB, false)
	ms.NoError(ms.DB.Destroy(&tempClaim.ClaimItems[0]),
		"error trying to destroy ClaimItem fixture for test")

	tests := []struct {
		name            string
		claim           Claim
		wantErrContains string
		wantErrKey      api.ErrorKey
		wantErrCat      api.ErrorCategory
		wantStatus      api.ClaimStatus
	}{
		{
			name:            "bad start status",
			claim:           draftClaim,
			wantErrKey:      api.ErrorClaimStatus,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "invalid claim status for deny",
		},
		{
			name:            "claim with no ClaimItem",
			claim:           emptyClaim,
			wantErrKey:      api.ErrorClaimMissingClaimItem,
			wantErrCat:      api.CategoryUser,
			wantErrContains: "claim must have a claimItem if no longer in draft",
		},
		{
			name:       "from review1 to denied",
			claim:      review1Claim,
			wantStatus: api.ClaimStatusDenied,
		},
		{
			name:       "from review2 to denied",
			claim:      review2Claim,
			wantStatus: api.ClaimStatusDenied,
		},
		{
			name:       "from review3 to denied",
			claim:      review3Claim,
			wantStatus: api.ClaimStatusDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const message = "change all the things"
			ctx := CreateTestContext(admin)
			got := tt.claim.Deny(ctx, message)

			if tt.wantErrContains != "" {
				ms.Error(got, " did not return expected error")
				var appErr *api.AppError
				ms.True(errors.As(got, &appErr), "returned an error that is not an AppError")
				ms.Contains(got.Error(), tt.wantErrContains, "error message is not correct")
				ms.Equal(appErr.Key, tt.wantErrKey, "error key is not correct")
				ms.Equal(appErr.Category, tt.wantErrCat, "error category is not correct")
				return
			}
			ms.NoError(got)

			ms.Equal(tt.wantStatus, tt.claim.Status, "incorrect status")
			ms.Equal(admin.ID.String(), tt.claim.ReviewerID.UUID.String(), "incorrect reviewer id")
			ms.WithinDuration(time.Now().UTC(), tt.claim.ReviewDate.Time, time.Second*2, "incorrect reviewer date id")
			ms.Equal(message, tt.claim.StatusReason, "incorrect status reason message")
		})
	}
}

func (ms *ModelSuite) TestClaim_HasReceiptFile() {
	db := ms.DB
	config := FixturesConfig{
		NumberOfPolicies: 3,
		ItemsPerPolicy:   1,
		ClaimsPerPolicy:  1,
	}
	fixtures := CreateItemFixtures(db, config)

	files := CreateFileFixtures(db, 2, CreateAdminUsers(db)[AppRoleSteward].ID).Files

	policies := fixtures.Policies

	claimNoFile := UpdateClaimStatus(db, policies[0].Claims[0], api.ClaimStatusReceipt, "")
	claimNoReceiptFile := UpdateClaimStatus(db, policies[1].Claims[0], api.ClaimStatusReceipt, "")
	claimWithReceipt := UpdateClaimStatus(db, policies[2].Claims[0], api.ClaimStatusReceipt, "")

	ms.NoError(NewClaimFile(claimNoReceiptFile.ID, files[0].ID, api.ClaimFilePurposeRepairEstimate).Create(db))
	ms.NoError(NewClaimFile(claimWithReceipt.ID, files[1].ID, api.ClaimFilePurposeReceipt).Create(db))

	tests := []struct {
		name  string
		claim Claim
		want  bool
	}{
		{
			name:  "has no file at all",
			claim: claimNoFile,
			want:  false,
		},
		{
			name:  "has only a non-receipt file",
			claim: claimNoReceiptFile,
			want:  false,
		},
		{
			name:  "has a receipt file",
			claim: claimWithReceipt,
			want:  true,
		},
	}
	for _, tt := range tests {
		ms.T().Run(tt.name, func(t *testing.T) {
			got := tt.claim.HasReceiptFile(db)
			ms.Equal(tt.want, got, "incorrect value for whether claim has a receipt file")
		})
	}
}

func (ms *ModelSuite) TestClaim_SubmittedAt() {
	fixConfig := FixturesConfig{
		NumberOfPolicies:   2,
		ItemsPerPolicy:     4,
		ClaimsPerPolicy:    5,
		ClaimItemsPerClaim: 1,
	}

	fixtures := CreateItemFixtures(ms.DB, fixConfig)
	policy := fixtures.Policies[0]
	draftClaim := policy.Claims[0]
	review1Claim := policy.Claims[1]
	review2Claim := policy.Claims[2]
	user := policy.Members[0]

	// Create test history fixtures for the claims
	historyFixtures := ClaimHistories{
		{
			ClaimID: review1Claim.ID,
			UserID:  user.ID,
			Action:  api.HistoryActionCreate,
		},
		{
			ClaimID:   review1Claim.ID,
			UserID:    user.ID,
			Action:    api.HistoryActionUpdate,
			FieldName: FieldClaimItemLocation,
			OldValue:  "USA",
			NewValue:  "UK",
		},
		{ // This is the history that should be used for the SubmittedAt time
			ClaimID:   review1Claim.ID,
			UserID:    user.ID,
			Action:    api.HistoryActionUpdate,
			FieldName: FieldClaimStatus,
			OldValue:  string(api.ClaimStatusDraft),
			NewValue:  string(api.ClaimStatusReview1),
		},
		{
			ClaimID:   review1Claim.ID,
			UserID:    user.ID,
			Action:    api.HistoryActionUpdate,
			FieldName: FieldClaimStatus,
			OldValue:  string(api.ClaimStatusReview1),
			NewValue:  string(api.ClaimStatusRevision),
		},
		{
			ClaimID:   review1Claim.ID,
			UserID:    user.ID,
			Action:    api.HistoryActionUpdate,
			FieldName: FieldClaimStatus,
			OldValue:  string(api.ClaimStatusRevision),
			NewValue:  string(api.ClaimStatusReview1),
		},
		{
			ClaimID: review2Claim.ID,
			UserID:  user.ID,
			Action:  api.HistoryActionCreate,
		},
		{
			ClaimID:   review2Claim.ID,
			UserID:    user.ID,
			Action:    api.HistoryActionUpdate,
			FieldName: FieldClaimItemLocation,
			OldValue:  "France",
			NewValue:  "Germany",
		},
		{ // This is the history that should be used for the SubmittedAt time
			ClaimID:   review2Claim.ID,
			UserID:    user.ID,
			Action:    api.HistoryActionUpdate,
			FieldName: FieldClaimStatus,
			OldValue:  string(api.ClaimStatusDraft),
			NewValue:  string(api.ClaimStatusReview1),
		},
		{
			ClaimID:   review2Claim.ID,
			UserID:    user.ID,
			Action:    api.HistoryActionUpdate,
			FieldName: FieldClaimStatus,
			OldValue:  string(api.ClaimStatusReview1),
			NewValue:  string(api.ClaimStatusReview2),
		},
	}

	for i := range historyFixtures {
		MustCreate(ms.DB, &historyFixtures[i])
	}

	// The history entries that should be used for the SubmittedAt time
	review1History := historyFixtures[2]
	review2History := historyFixtures[7]

	tests := []struct {
		name  string
		claim Claim
		want  time.Time
	}{
		{
			name:  "draft Claim",
			claim: draftClaim,
			want:  draftClaim.UpdatedAt,
		},
		{
			name:  "review1Claim",
			claim: review1Claim,
			want:  review1History.CreatedAt,
		},
		{
			name:  "has a receipt file",
			claim: review2Claim,
			want:  review2History.CreatedAt,
		},
	}

	for _, tt := range tests {
		ms.T().Run(tt.name, func(t *testing.T) {
			got := tt.claim.SubmittedAt(ms.DB)
			ms.WithinDuration(tt.want, got, time.Duration(1), "incorrect SubmittedAt")
		})
	}
}

func (ms *ModelSuite) TestClaim_ConvertToAPI() {
	fixtures := CreateItemFixtures(ms.DB, FixturesConfig{
		ClaimsPerPolicy:    1,
		ClaimItemsPerClaim: 2,
		ClaimFilesPerClaim: 3,
	})
	claim := fixtures.Claims[0]

	claim.StatusReason = "change request " + domain.RandomString(8, "0123456789")

	got := claim.ConvertToAPI(ms.DB)

	ms.Equal(claim.ID, got.ID, "ID is not correct")
	ms.Equal(claim.PolicyID, got.PolicyID, "PolicyID is not correct")
	ms.Equal(claim.ReferenceNumber, got.ReferenceNumber, "ReferenceNumber is not correct")
	ms.Equal(claim.IncidentDate, got.IncidentDate, "IncidentDate is not correct")
	ms.Equal(claim.IncidentType, got.IncidentType, "IncidentType is not correct")
	ms.Equal(claim.IncidentDescription, got.IncidentDescription, "IncidentDescription is not correct")
	ms.Equal(claim.Status, got.Status, "Status is not correct")
	ms.EqualNullTime(claim.ReviewDate, got.ReviewDate, "ReviewDate is not correct")
	ms.EqualNullUUID(claim.ReviewerID, got.ReviewerID, "ReviewerID is not correct")
	ms.EqualNullTime(claim.PaymentDate, got.PaymentDate, "PaymentDate is not correct")
	ms.Equal(claim.TotalPayout, got.TotalPayout, "TotalPayout is not correct")
	ms.Equal(claim.StatusReason, got.StatusReason, "StatusReason is not correct")

	ms.Greater(len(claim.ClaimItems), 0, "test should be revised, fixture has no ClaimItems")
	ms.Len(got.Items, len(claim.ClaimItems), "Items is not correct length")

	ms.Greater(len(claim.ClaimFiles), 0, "test should be revised, fixture has no ClaimFiles")
	ms.Len(got.Files, len(claim.ClaimFiles), "Files is not correct length")
}

func (ms *ModelSuite) TestClaim_Compare() {
	f := CreateItemFixtures(ms.DB, FixturesConfig{ClaimsPerPolicy: 1})
	oldClaim := f.Claims[0]
	newClaim := Claim{
		ReviewDate:   nulls.NewTime(time.Now().UTC().Add(-1 * time.Hour)),
		ReviewerID:   nulls.NewUUID(f.Users[0].ID),
		PaymentDate:  nulls.NewTime(time.Now().UTC()),
		TotalPayout:  10000,
		StatusReason: "because",
	}

	tests := []struct {
		name string
		new  Claim
		old  Claim
		want []FieldUpdate
	}{
		{
			name: "1",
			new:  newClaim,
			old:  oldClaim,
			want: []FieldUpdate{
				{
					FieldName: FieldClaimPolicyID,
					OldValue:  oldClaim.PolicyID.String(),
					NewValue:  newClaim.PolicyID.String(),
				},
				{
					FieldName: FieldClaimReferenceNumber,
					OldValue:  oldClaim.ReferenceNumber,
					NewValue:  newClaim.ReferenceNumber,
				},
				{
					FieldName: FieldClaimIncidentDate,
					OldValue:  oldClaim.IncidentDate.String(),
					NewValue:  newClaim.IncidentDate.String(),
				},
				{
					FieldName: FieldClaimIncidentType,
					OldValue:  string(oldClaim.IncidentType),
					NewValue:  string(newClaim.IncidentType),
				},
				{
					FieldName: FieldClaimIncidentDescription,
					OldValue:  oldClaim.IncidentDescription,
					NewValue:  newClaim.IncidentDescription,
				},
				{
					FieldName: FieldClaimStatus,
					OldValue:  string(oldClaim.Status),
					NewValue:  string(newClaim.Status),
				},
				{
					FieldName: FieldClaimReviewDate,
					OldValue:  oldClaim.ReviewDate.Time.String(),
					NewValue:  newClaim.ReviewDate.Time.String(),
				},
				{
					FieldName: FieldClaimReviewerID,
					OldValue:  oldClaim.ReviewerID.UUID.String(),
					NewValue:  newClaim.ReviewerID.UUID.String(),
				},
				{
					FieldName: FieldClaimPaymentDate,
					OldValue:  oldClaim.PaymentDate.Time.String(),
					NewValue:  newClaim.PaymentDate.Time.String(),
				},
				{
					FieldName: FieldClaimTotalPayout,
					OldValue:  oldClaim.TotalPayout.String(),
					NewValue:  newClaim.TotalPayout.String(),
				},
				{
					FieldName: FieldClaimStatusReason,
					OldValue:  oldClaim.StatusReason,
					NewValue:  newClaim.StatusReason,
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

func (ms *ModelSuite) TestClaim_NewHistory() {
	f := CreateItemFixtures(ms.DB, FixturesConfig{ClaimsPerPolicy: 1})
	claim := f.Claims[0]
	user := f.Users[0]

	const newStatus = api.ClaimStatusApproved
	newIncidentDate := time.Now().UTC()

	tests := []struct {
		name   string
		claim  Claim
		user   User
		update FieldUpdate
		want   ClaimHistory
	}{
		{
			name:  "Status",
			claim: claim,
			user:  user,
			update: FieldUpdate{
				FieldName: "Status",
				OldValue:  string(claim.Status),
				NewValue:  string(newStatus),
			},
			want: ClaimHistory{
				ClaimID:   claim.ID,
				UserID:    user.ID,
				Action:    api.HistoryActionUpdate,
				FieldName: "Status",
				OldValue:  string(claim.Status),
				NewValue:  string(newStatus),
			},
		},
		{
			name:  "IncidentDate",
			claim: claim,
			user:  user,
			update: FieldUpdate{
				FieldName: "IncidentDate",
				OldValue:  claim.IncidentDate.String(),
				NewValue:  newIncidentDate.String(),
			},
			want: ClaimHistory{
				ClaimID:   claim.ID,
				UserID:    user.ID,
				Action:    api.HistoryActionUpdate,
				FieldName: "IncidentDate",
				OldValue:  claim.IncidentDate.String(),
				NewValue:  newIncidentDate.String(),
			},
		},
	}
	for _, tt := range tests {
		ms.T().Run(tt.name, func(t *testing.T) {
			got := tt.claim.NewHistory(CreateTestContext(tt.user), api.HistoryActionUpdate, tt.update)
			ms.False(tt.want.NewValue == tt.want.OldValue, "test isn't correctly checking a field update")
			ms.Equal(tt.want.ClaimID, got.ClaimID, "ClaimID is not correct")
			ms.Equal(tt.want.UserID, got.UserID, "UserID is not correct")
			ms.Equal(tt.want.Action, got.Action, "Action is not correct")
			ms.Equal(tt.want.FieldName, got.FieldName, "FieldName is not correct")
			ms.Equal(tt.want.OldValue, got.OldValue, "OldValue is not correct")
			ms.Equal(tt.want.NewValue, got.NewValue, "NewValue is not correct")
		})
	}
}

func (ms *ModelSuite) Test_ClaimsWithRecentStatusChanges() {
	fixtures := CreateClaimHistoryFixtures_RecentClaimStatusChanges(ms.DB)
	chFixes := fixtures.ClaimHistories

	gotRaw, gotErr := ClaimsWithRecentStatusChanges(ms.DB)
	ms.NoError(gotErr)

	const tmFmt = "Jan _2 15:04:05.00"

	got := make([][2]string, len(gotRaw))
	for i, g := range gotRaw {
		got[i] = [2]string{g.Claim.ID.String(), g.StatusUpdatedAt.Format(tmFmt)}
	}

	want := [][2]string{
		{chFixes[3].ClaimID.String(), chFixes[3].UpdatedAt.Format(tmFmt)},
		{chFixes[7].ClaimID.String(), chFixes[7].UpdatedAt.Format(tmFmt)},
	}

	ms.ElementsMatch(want, got, "incorrect results")
}

func (ms *ModelSuite) TestClaim_CreateLedgerEntry() {
	f := CreateItemFixtures(ms.DB, FixturesConfig{ClaimsPerPolicy: 1, ClaimItemsPerClaim: 1})
	item := f.Claims[0].ClaimItems[0].Item

	user := f.Users[0]
	ctx := CreateTestContext(user)
	ms.NoError(item.SetAccountablePerson(ms.DB, user.ID))
	ms.NoError(item.Update(ctx))

	var claim Claim
	ms.NoError(ms.DB.Find(&claim, f.Claims[0].ID))

	ms.Error(claim.CreateLedgerEntry(ms.DB), "expected an error, claim isn't approved yet")

	claim.Status = api.ClaimStatusApproved
	claim.TotalPayout = 12345
	ms.NoError(ms.DB.Update(&claim), "unable to update claim test fixture")

	ms.NoError(claim.CreateLedgerEntry(ms.DB), "claim is approved now, it shouldn't be a problem")

	var le LedgerEntry
	ms.NoError(ms.DB.Where("claim_id = ?", claim.ID).First(&le))

	ms.Equal(LedgerEntryTypeClaim, le.Type, "Type is incorrect")
	ms.Equal(item.PolicyID, le.PolicyID, "PolicyID is incorrect")
	ms.Equal(item.ID, le.ItemID.UUID, "ItemID is incorrect")
	ms.Equal(claim.ID, le.ClaimID.UUID, "ClaimID is incorrect")
	ms.Equal(api.Currency(-12345), le.Amount, "Amount is incorrect")
	ms.Equal(user.FirstName, le.FirstName, "FirstName is incorrect")
	ms.Equal(user.LastName, le.LastName, "LastName is incorrect")
}

func (ms *ModelSuite) TestClaims_ByStatus() {
	f := CreateItemFixtures(ms.DB, FixturesConfig{
		NumberOfPolicies: 9,
		ClaimsPerPolicy:  1,
	})

	f.Claims[0].Status = api.ClaimStatusDraft
	f.Claims[1].Status = api.ClaimStatusReview1
	f.Claims[2].Status = api.ClaimStatusReview2
	f.Claims[3].Status = api.ClaimStatusReview3
	f.Claims[4].Status = api.ClaimStatusRevision
	f.Claims[5].Status = api.ClaimStatusReceipt
	f.Claims[6].Status = api.ClaimStatusApproved
	f.Claims[7].Status = api.ClaimStatusPaid
	f.Claims[8].Status = api.ClaimStatusDenied

	ms.NoError(ms.DB.Update(&f.Claims))

	tests := []struct {
		name         string
		statuses     []api.ClaimStatus
		wantClaimIDs []uuid.UUID
		wantErr      bool
	}{
		{
			name:         "default",
			wantClaimIDs: []uuid.UUID{f.Claims[1].ID, f.Claims[2].ID, f.Claims[3].ID},
			wantErr:      false,
		},
		{
			name:         "approved and paid",
			statuses:     []api.ClaimStatus{api.ClaimStatusApproved, api.ClaimStatusPaid},
			wantClaimIDs: []uuid.UUID{f.Claims[6].ID, f.Claims[7].ID},
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		ms.T().Run(tt.name, func(t *testing.T) {
			var claims Claims
			err := claims.ByStatus(ms.DB, tt.statuses)
			ms.NoError(err)

			gotIDs := make([]uuid.UUID, len(claims))
			for i := range claims {
				gotIDs[i] = claims[i].ID
			}

			ms.Equal(len(tt.wantClaimIDs), len(gotIDs))
			ms.ElementsMatch(tt.wantClaimIDs, gotIDs)
		})
	}
}

func (ms *ModelSuite) TestClaim_calculatePayout() {
	fixtures := CreateItemFixtures(ms.DB, FixturesConfig{ClaimsPerPolicy: 1, ClaimItemsPerClaim: 1})
	fixtures.Claims[0].ClaimItems[0].RepairEstimate = 100
	ms.NoError(ms.DB.Update(&fixtures.Claims[0].ClaimItems[0]))

	// Get a fresh copy of the claim to ensure the UUT hydrates it as necessary
	var claim Claim
	ms.NoError(claim.FindByID(ms.DB, fixtures.Claims[0].ID))

	before := claim.TotalPayout

	ms.NoError(claim.calculatePayout(CreateTestContext(fixtures.Users[0])))

	// The claim item test will check the actual amount. Just make sure it changed.
	ms.False(claim.TotalPayout == before, "payout was not updated")
}

func (ms *ModelSuite) TestClaim_Create() {
	f := CreateItemFixtures(ms.DB, FixturesConfig{})

	tests := []struct {
		name     string
		claim    Claim
		appError *api.AppError
	}{
		{
			name:     "need Policy ID",
			claim:    Claim{},
			appError: &api.AppError{Category: api.CategoryUser, Key: api.ErrorValidation},
		},
		{
			name:     "minimum",
			claim:    Claim{PolicyID: f.Policies[0].ID},
			appError: nil,
		},
	}
	for _, tt := range tests {
		ms.T().Run(tt.name, func(t *testing.T) {
			err := tt.claim.Create(ms.DB)
			if tt.appError != nil {
				ms.Error(err, "test should have produced an error")
				ms.EqualAppError(*tt.appError, err)
				return
			}
			ms.NoError(err)
		})
	}
}
