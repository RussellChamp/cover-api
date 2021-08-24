package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/silinternational/riskman-api/domain"

	"github.com/silinternational/riskman-api/api"
	"github.com/silinternational/riskman-api/models"
)

func (as *ActionSuite) Test_ClaimsList() {
	const numberOfPolicies = 3
	const claimsPerPolicy = 4
	const totalNumberOfClaims = claimsPerPolicy * numberOfPolicies
	fixConfig := models.FixturesConfig{
		NumberOfPolicies:    numberOfPolicies,
		UsersPerPolicy:      1,
		ClaimsPerPolicy:     claimsPerPolicy,
		ClaimItemsPerClaim:  2,
		DependentsPerPolicy: 0,
		ItemsPerPolicy:      2,
	}

	fixtures := models.CreateItemFixtures(as.DB, fixConfig)

	// alias a couple users
	appAdmin := fixtures.Policies[0].Members[0]
	normalUser := fixtures.Policies[1].Members[0]

	// make an admin
	appAdmin.AppRole = models.AppRoleAdmin
	err := appAdmin.Update(as.DB)
	as.NoError(err, "failed to make an app admin")

	tests := []struct {
		name          string
		actor         models.User
		wantStatus    int
		wantClaims    int
		wantInBody    string
		notWantInBody string
	}{
		{
			name:          "normal user",
			actor:         normalUser,
			wantStatus:    http.StatusOK,
			wantClaims:    claimsPerPolicy,
			wantInBody:    fixtures.Policies[1].Claims[0].ID.String(),
			notWantInBody: fixtures.Policies[0].Claims[0].ID.String(),
		},
		{
			name:       "admin user",
			actor:      appAdmin,
			wantStatus: http.StatusOK,
			wantClaims: totalNumberOfClaims,
			wantInBody: fixtures.Policies[0].Claims[0].ID.String(),
		},
	}

	for _, tt := range tests {
		as.T().Run(tt.name, func(t *testing.T) {
			req := as.JSON("/claims")
			req.Headers["Authorization"] = fmt.Sprintf("Bearer %s", tt.actor.Email)
			req.Headers["content-type"] = "application/json"
			res := req.Get()

			body := res.Body.String()
			as.Equal(tt.wantStatus, res.Code, "incorrect status code returned, body: %s", body)
			if tt.wantInBody != "" {
				as.Contains(body, tt.wantInBody)
			}
			if tt.notWantInBody != "" {
				as.NotContains(body, tt.notWantInBody)
			}

			if res.Code != http.StatusOK {
				return
			}
			var responseObject api.Claims
			as.NoError(json.Unmarshal([]byte(body), &responseObject))
			as.Len(responseObject, tt.wantClaims, "incorrect # of claims, %+v", responseObject)
			for _, c := range responseObject {
				as.Len(c.Items, fixConfig.ItemsPerPolicy)
			}
		})
	}
}

func (as *ActionSuite) Test_ClaimsView() {
	fixConfig := models.FixturesConfig{
		NumberOfPolicies:    3,
		UsersPerPolicy:      1,
		ClaimsPerPolicy:     4,
		DependentsPerPolicy: 0,
		ItemsPerPolicy:      2,
	}

	fixtures := models.CreateItemFixtures(as.DB, fixConfig)

	// alias a couple users
	appAdmin := fixtures.Policies[0].Members[0]
	firstUser := fixtures.Policies[1].Members[0]
	secondUser := fixtures.Policies[2].Members[0]

	// make an admin
	appAdmin.AppRole = models.AppRoleAdmin
	err := appAdmin.Update(as.DB)
	as.NoError(err, "failed to make an app admin")

	tests := []struct {
		name          string
		actor         models.User
		claim         models.Claim
		wantStatus    int
		wantInBody    string
		notWantInBody string
	}{
		{
			name:          "unauthorized user",
			actor:         firstUser,
			claim:         fixtures.Policies[2].Claims[0],
			wantStatus:    http.StatusNotFound,
			notWantInBody: fixtures.Policies[2].ID.String(),
		},
		{
			name:       "authorized user",
			actor:      secondUser,
			claim:      fixtures.Policies[2].Claims[0],
			wantStatus: http.StatusOK,
			wantInBody: fixtures.Policies[2].Claims[0].ID.String(),
		},
		{
			name:       "admin user",
			actor:      appAdmin,
			claim:      fixtures.Policies[2].Claims[0],
			wantStatus: http.StatusOK,
			wantInBody: fixtures.Policies[2].Claims[0].ID.String(),
		},
	}

	for _, tt := range tests {
		as.T().Run(tt.name, func(t *testing.T) {
			req := as.JSON("/claims/" + tt.claim.ID.String())
			req.Headers["Authorization"] = fmt.Sprintf("Bearer %s", tt.actor.Email)
			req.Headers["content-type"] = "application/json"
			res := req.Get()

			body := res.Body.String()
			as.Equal(tt.wantStatus, res.Code, "incorrect status code returned, body: %s", body)
			if tt.wantInBody != "" {
				as.Contains(body, tt.wantInBody, "did not find expected string")
			}
			if tt.notWantInBody != "" {
				as.NotContains(body, tt.notWantInBody, "found unexpected string")
			}

			if res.Code != http.StatusOK {
				return
			}
			var responseObject api.Claim
			as.NoError(json.Unmarshal([]byte(body), &responseObject))
			as.Equal(tt.claim.ID, responseObject.ID, "incorrect object in response", responseObject)
		})
	}
}

func (as *ActionSuite) Test_ClaimsUpdate() {
	fixConfig := models.FixturesConfig{
		NumberOfPolicies:    3,
		UsersPerPolicy:      1,
		ClaimsPerPolicy:     4,
		DependentsPerPolicy: 0,
		ItemsPerPolicy:      2,
	}

	fixtures := models.CreateItemFixtures(as.DB, fixConfig)

	// alias a couple users
	appAdmin := fixtures.Policies[0].Members[0]
	firstUser := fixtures.Policies[1].Members[0]
	secondUser := fixtures.Policies[2].Members[0]

	// make an admin
	appAdmin.AppRole = models.AppRoleAdmin
	err := appAdmin.Update(as.DB)
	as.NoError(err, "failed to make an app admin")

	input := api.ClaimUpdateInput{
		EventDate:        time.Now().UTC(),
		EventType:        api.ClaimEventTypeTheft,
		EventDescription: "a description",
	}

	tests := []struct {
		name          string
		actor         models.User
		claim         models.Claim
		input         api.ClaimUpdateInput
		wantStatus    int
		wantInBody    string
		notWantInBody string
	}{
		{
			name:          "unauthorized user",
			actor:         firstUser,
			claim:         fixtures.Policies[2].Claims[0],
			input:         input,
			wantStatus:    http.StatusNotFound,
			notWantInBody: fixtures.Policies[2].ID.String(),
		},
		{
			name:       "authorized user",
			actor:      secondUser,
			claim:      fixtures.Policies[2].Claims[0],
			input:      input,
			wantStatus: http.StatusOK,
			wantInBody: fixtures.Policies[2].Claims[0].ID.String(),
		},
		{
			name:       "admin user",
			actor:      appAdmin,
			claim:      fixtures.Policies[2].Claims[0],
			input:      input,
			wantStatus: http.StatusOK,
			wantInBody: fixtures.Policies[2].Claims[0].ID.String(),
		},
	}

	for _, tt := range tests {
		as.T().Run(tt.name, func(t *testing.T) {
			req := as.JSON("/claims/" + tt.claim.ID.String())
			req.Headers["Authorization"] = fmt.Sprintf("Bearer %s", tt.actor.Email)
			req.Headers["content-type"] = "application/json"
			res := req.Put(tt.input)

			body := res.Body.String()
			as.Equal(tt.wantStatus, res.Code, "incorrect status code returned, body: %s", body)
			if tt.wantInBody != "" {
				as.Contains(body, tt.wantInBody, "did not find expected string")
			}
			if tt.notWantInBody != "" {
				as.NotContains(body, tt.notWantInBody, "found unexpected string")
			}

			if res.Code != http.StatusOK {
				return
			}
			var responseObject api.Claim
			as.NoError(json.Unmarshal([]byte(body), &responseObject))
			as.Equal(tt.claim.ID, responseObject.ID, "incorrect object in response", responseObject)

			updatedClaim := models.Claim{}
			as.NoError(as.DB.Find(&updatedClaim, tt.claim.ID))
			as.verifyClaimUpdate(input, updatedClaim)
		})
	}
}

func (as *ActionSuite) verifyClaimUpdate(input api.ClaimUpdateInput, claim models.Claim) {
	as.Equal(input.EventType, claim.EventType, "EventType not correct")
	as.Equal(input.EventDescription, claim.EventDescription, "EventDescription not correct")
	as.WithinDuration(input.EventDate, claim.EventDate, time.Millisecond, "EventDate not correct")
}

func (as *ActionSuite) Test_ClaimsCreate() {
	fixConfig := models.FixturesConfig{
		NumberOfPolicies:    3,
		UsersPerPolicy:      1,
		DependentsPerPolicy: 0,
		ItemsPerPolicy:      2,
	}

	fixtures := models.CreateItemFixtures(as.DB, fixConfig)

	policyByOther := fixtures.Policies[0]
	policyByUser := fixtures.Policies[1]
	policyByAdmin := fixtures.Policies[2]

	// alias a couple users
	appAdmin := fixtures.Policies[2].Members[0]
	normalUser := policyByUser.Members[0]

	// make an admin
	appAdmin.AppRole = models.AppRoleAdmin
	err := appAdmin.Update(as.DB)
	as.NoError(err, "failed to make an app admin")

	input := api.ClaimCreateInput{
		EventDate:        time.Now(),
		EventType:        api.ClaimEventTypeTheft,
		EventDescription: "a description",
	}

	tests := []struct {
		name          string
		actor         models.User
		policy        models.Policy
		input         api.ClaimCreateInput
		wantStatus    int
		wantInBody    []string
		notWantInBody string
	}{
		{
			name:          "incomplete input",
			actor:         normalUser,
			policy:        policyByUser,
			input:         api.ClaimCreateInput{},
			wantStatus:    http.StatusBadRequest,
			notWantInBody: policyByUser.ID.String(),
		},
		{
			name:       "valid input",
			actor:      normalUser,
			policy:     policyByUser,
			input:      input,
			wantStatus: http.StatusOK,
			wantInBody: []string{
				`"policy_id":"` + policyByUser.ID.String(),
				`"event_type":"` + string(input.EventType),
				`"event_description":"` + input.EventDescription,
				`"status":"` + string(api.ClaimStatusDraft),
				`"claim_items":[]`,
			},
		},
		{
			name:          "other person's policy",
			actor:         normalUser,
			policy:        policyByOther,
			input:         input,
			wantStatus:    http.StatusNotFound,
			notWantInBody: policyByOther.ID.String(),
		},
		{
			name:       "admin operation on other person's policy",
			actor:      appAdmin,
			policy:     policyByAdmin,
			input:      input,
			wantStatus: http.StatusOK,
			wantInBody: []string{
				`"policy_id":"` + policyByAdmin.ID.String(),
				`"event_type":"` + string(input.EventType),
				`"event_description":"` + input.EventDescription,
				`"status":"` + string(api.ClaimStatusDraft),
				`"claim_items":[]`,
			},
		},
	}

	for _, tt := range tests {
		as.T().Run(tt.name, func(t *testing.T) {
			req := as.JSON(fmt.Sprintf("/policies/%s/claims", tt.policy.ID))
			req.Headers["Authorization"] = fmt.Sprintf("Bearer %s", tt.actor.Email)
			req.Headers["content-type"] = "application/json"
			res := req.Post(tt.input)

			body := res.Body.String()
			as.Equal(tt.wantStatus, res.Code, "incorrect status code returned, body: %s", body)

			as.verifyResponseData(tt.wantInBody, body, "Create Claim fields")

			if tt.notWantInBody != "" {
				as.NotContains(body, tt.notWantInBody)
			}

			if res.Code != http.StatusOK {
				return
			}
			var respObj api.Claim
			as.NoError(json.Unmarshal([]byte(body), &respObj))

			as.Equal(tt.input.EventDescription, respObj.EventDescription,
				"response object is not correct, %+v", respObj)
		})
	}
}

// TODO make this test more robust
func (as *ActionSuite) Test_ClaimsItemsCreate() {

	fixConfig := models.FixturesConfig{
		NumberOfPolicies:    3,
		UsersPerPolicy:      1,
		DependentsPerPolicy: 0,
		ItemsPerPolicy:      2,
		ClaimsPerPolicy:     1,
	}

	fixtures := models.CreateItemFixtures(as.DB, fixConfig)

	claim := fixtures.Policies[1].Claims[0]
	item := fixtures.Policies[1].Items[0]

	otherUser := fixtures.Policies[0].Members[0]
	sameUser := fixtures.Policies[1].Members[0]

	input := api.ClaimItemCreateInput{
		ItemID:          item.ID,
		IsRepairable:    true,
		RepairEstimate:  200,
		RepairActual:    0,
		ReplaceEstimate: 300,
		ReplaceActual:   0,
		PayoutOption:    "my account",
		PayoutAmount:    0,
		FMV:             250,
	}

	tests := []struct {
		name          string
		actor         models.User
		claim         models.Claim
		input         api.ClaimItemCreateInput
		wantStatus    int
		wantInBody    []string
		notWantInBody string
	}{
		{
			name:          "incomplete input",
			actor:         sameUser,
			claim:         claim,
			input:         api.ClaimItemCreateInput{},
			wantStatus:    http.StatusNotFound,
			wantInBody:    []string{api.ErrorResourceNotFound.String()},
			notWantInBody: claim.ID.String(),
		},
		{
			name:       "valid input",
			actor:      sameUser,
			claim:      claim,
			input:      input,
			wantStatus: http.StatusOK,
			wantInBody: []string{
				`"item_id":"` + input.ItemID.String(),
				`"name":"` + item.Name,
				fmt.Sprintf(`"in_storage":%t`, item.InStorage),
				`"country":"` + item.Country,
				`"description":"` + item.Description,
				`"policy_id":"` + item.PolicyID.String(),
				`"make":"` + item.Make,
				`"model":"` + item.Model,
				`"serial_number":"` + item.SerialNumber,
				fmt.Sprintf(`"coverage_amount":%v`, item.CoverageAmount),
				`"purchase_date":"` + item.PurchaseDate.Format(domain.DateFormat),
				`"coverage_status":"` + string(item.CoverageStatus),
				`"coverage_start_date":"` + item.CoverageStartDate.Format(domain.DateFormat),
				`"category":{"id":"` + item.CategoryID.String(),
				`"claim_id":"` + claim.ID.String(),
				`"status":"` + string(api.ClaimItemStatusDraft),
				fmt.Sprintf(`"is_repairable":%t`, input.IsRepairable),
				fmt.Sprintf(`"repair_estimate":%v`, input.RepairEstimate),
				fmt.Sprintf(`"replace_estimate":%v`, input.ReplaceEstimate),
				`"payout_option":"` + input.PayoutOption,
				fmt.Sprintf(`"fmv":%v`, input.FMV),
			},
		},
		{
			name:          "other person's policy",
			actor:         otherUser,
			claim:         claim,
			input:         input,
			wantStatus:    http.StatusNotFound,
			notWantInBody: claim.ID.String(),
		},
	}

	for _, tt := range tests {
		as.T().Run(tt.name, func(t *testing.T) {
			req := as.JSON(fmt.Sprintf("/%s/%s/%s", domain.TypeClaim, tt.claim.ID, domain.TypeItem))
			req.Headers["Authorization"] = fmt.Sprintf("Bearer %s", tt.actor.Email)
			req.Headers["content-type"] = "application/json"

			res := req.Post(tt.input)

			body := res.Body.String()
			as.Equal(tt.wantStatus, res.Code, "incorrect status code returned, body: %s", body)

			as.verifyResponseData(tt.wantInBody, body, "CreateItem Claim fields")

			if tt.notWantInBody != "" {
				as.NotContains(body, tt.notWantInBody)
			}

			if res.Code != http.StatusOK {
				return
			}
			var respObj api.ClaimItem
			as.NoError(json.Unmarshal([]byte(body), &respObj))

			as.Equal(tt.input.PayoutOption, respObj.PayoutOption,
				"response object is not correct, %+v", respObj)
		})
	}

}
